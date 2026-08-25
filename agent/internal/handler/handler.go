package handler

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"buf.build/go/protovalidate"

	"github.com/manchtools/cadestro/agent/internal/executor"
	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/inventory"
	syslog "github.com/manchtools/cadestro/sdk/sys/log"
	"github.com/manchtools/cadestro/sdk/sys/osquery"
)

const heartbeatQueryTimeout = 10 * time.Second

var streamValidator = protovalidate.GlobalValidator

var handlerRunner = func() sysexec.Runner {
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		panic("handler: Direct runner must construct: " + err.Error())
	}
	return r
}()

type Handler struct {
	logger       *slog.Logger
	executor     *executor.Executor
	osquery      osquery.Querier
	store        *store.Store
	syncTrigger  chan<- struct{}
	mu           sync.Mutex
	connectedCh  chan struct{}
	connectedSet bool

	terminalSender         TerminalSender
	terminals              map[string]*terminalSession
	terminalLimit          int
	terminalIdleTimeout    time.Duration
	terminalSweeperStarted bool
	terminalSweeperStop    chan struct{}

	now func() time.Time
}

func NewHandler(logger *slog.Logger, exec *executor.Executor, st *store.Store, syncTrigger chan<- struct{}) *Handler {
	return &Handler{
		logger:      logger,
		executor:    exec,
		store:       st,
		syncTrigger: syncTrigger,
		connectedCh: make(chan struct{}),
		now:         time.Now,
	}
}

func (h *Handler) OnSyncDevice(context.Context, *pb.SyncDeviceCommand) error {
	if h.syncTrigger == nil {
		return nil
	}
	select {
	case h.syncTrigger <- struct{}{}:
	default:
	}
	return nil
}

func (h *Handler) OnRebootDevice(ctx context.Context, _ *pb.RebootDeviceCommand) error {
	if h.executor == nil {
		return errors.New("executor is unavailable")
	}
	return h.executor.Reboot(ctx)
}

func (h *Handler) getOsquery() osquery.Querier {
	h.mu.Lock()
	if h.osquery != nil {
		r := h.osquery
		h.mu.Unlock()
		return r
	}
	h.mu.Unlock()

	registry, err := osquery.New(handlerRunner)
	if err != nil {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.osquery == nil {
		h.osquery = registry
		h.logger.Info("osquery detected and initialized")
	}
	return h.osquery
}

func (h *Handler) OnWelcome(ctx context.Context, welcome *pb.Welcome) error {
	h.logger.Info("received welcome from server", "server_version", welcome.ServerVersion)

	h.mu.Lock()
	if !h.connectedSet {
		close(h.connectedCh)
		h.connectedSet = true
	}
	h.mu.Unlock()

	return nil
}

func (h *Handler) WaitConnected(ctx context.Context) error {
	h.mu.Lock()
	ch := h.connectedCh
	h.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Handler) ResetConnection() {
	h.mu.Lock()
	if h.connectedSet {
		h.connectedCh = make(chan struct{})
		h.connectedSet = false
	}
	h.mu.Unlock()
}

const maxLogOutputBytes = 256

func sanitizeForLog(s string) string {
	if s == "" {
		return s
	}

	if strings.Contains(s, "enc:v1:") {
		s = redactEncMarkers(s)
	}
	if len(s) > maxLogOutputBytes {
		s = s[:maxLogOutputBytes] + "... [truncated by agent log filter]"
	}
	return s
}

func redactEncMarkers(s string) string {
	const marker = "enc:v1:"
	var out strings.Builder
	out.Grow(len(s))
	for {
		idx := strings.Index(s, marker)
		if idx < 0 {
			out.WriteString(s)
			return out.String()
		}
		out.WriteString(s[:idx])
		out.WriteString("[REDACTED-ENC]")

		i := idx + len(marker)
		for i < len(s) && isBase64Char(s[i]) {
			i++
		}
		s = s[i:]
	}
}

func isBase64Char(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '+' || b == '/' || b == '='
}

func (h *Handler) OnQuery(ctx context.Context, query *pb.OSQuery) (*pb.OSQueryResult, error) {
	queryID := query.GetQueryId().GetValue()
	h.logger.Info("received query", "query_id", queryID, "table", query.Table)

	if err := streamValidator.Validate(query); err != nil {
		h.logger.Warn("rejecting invalid query", "query_id", queryID, "error", err)
		return &pb.OSQueryResult{QueryId: query.GetQueryId(), Success: false, Error: err.Error()}, nil
	}

	oq := h.getOsquery()
	if oq == nil {
		h.logger.Warn("osquery not available", "query_id", queryID)
		return &pb.OSQueryResult{
			QueryId: query.QueryId,
			Success: false,
			Error:   "osquery is not installed on this system",
		}, nil
	}

	result, err := queryOsquery(ctx, oq, query)
	if err != nil {
		h.logger.Error("query execution error", "query_id", queryID, "error", err)
		return &pb.OSQueryResult{
			QueryId: query.QueryId,
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	h.logger.Info("query completed", "query_id", queryID, "success", result.Success, "row_count", len(result.Rows))
	return result, nil
}

func (h *Handler) OnError(ctx context.Context, err *pb.Error) error {
	h.logger.Error("received error from server", "code", err.Code, "message", err.Message)
	return nil
}

func (h *Handler) BuildHeartbeat() *pb.Heartbeat {
	hb := &pb.Heartbeat{}

	oq := h.getOsquery()
	if oq == nil {
		return hb
	}

	ctx, cancel := context.WithTimeout(context.Background(), heartbeatQueryTimeout)
	defer cancel()

	if result, _ := queryOsquery(ctx, oq, &pb.OSQuery{QueryId: &pb.QueryId{Value: "hb"}, Table: "uptime"}); result != nil && result.Success && len(result.Rows) > 0 {
		if sec, err := strconv.ParseInt(result.Rows[0].Data["total_seconds"], 10, 64); err == nil {
			hb.Uptime = durationpb.New(time.Duration(sec) * time.Second)
		}
	}

	if result, _ := queryOsquery(ctx, oq, &pb.OSQuery{QueryId: &pb.QueryId{Value: "hb"}, Table: "memory_info"}); result != nil && result.Success && len(result.Rows) > 0 {
		data := result.Rows[0].Data
		total, totalErr := strconv.ParseInt(data["memory_total"], 10, 64)
		free, freeErr := strconv.ParseInt(data["memory_free"], 10, 64)
		if totalErr != nil || freeErr != nil {

			slog.Debug("heartbeat: memory_info parse failed",
				"memory_total_err", totalErr, "memory_free_err", freeErr)
		} else if total > 0 {

			hb.MemoryPercent = float32(100 * (total - free) / total)
		}
	}

	return hb
}

func (h *Handler) OnRevokeLuksDeviceKey(ctx context.Context, req *pb.RevokeLuksDeviceKey) (bool, string) {
	actionID := req.GetActionId().GetValue()
	h.logger.Info("received LUKS device key revocation", "action_id", actionID)

	success, errMsg := h.executor.RevokeLuksDeviceKey(ctx, actionID)
	if !success {
		h.logger.Error("LUKS device key revocation failed", "action_id", actionID, "error", errMsg)
	} else {
		h.logger.Info("LUKS device key revoked", "action_id", actionID)
	}
	return success, errMsg
}

func (h *Handler) OnLogQuery(ctx context.Context, query *pb.LogQuery) (*pb.LogQueryResult, error) {
	queryID := query.GetQueryId().GetValue()
	h.logger.Info("received log query", "query_id", queryID, "unit", query.Unit)

	if err := streamValidator.Validate(query); err != nil {
		h.logger.Warn("rejecting invalid log query", "query_id", queryID, "error", err)
		return &pb.LogQueryResult{QueryId: query.GetQueryId(), Success: false, Error: err.Error()}, nil
	}

	src, err := syslog.New(syslog.Journald, handlerRunner)
	if err != nil {
		h.logger.Warn("log query setup failed", "query_id", queryID, "error", err)
		return &pb.LogQueryResult{QueryId: query.QueryId, Success: false, Error: err.Error()}, nil
	}
	lines, err := src.Query(ctx, syslog.Query{
		Unit:     query.Unit,
		Since:    query.Since,
		Until:    query.Until,
		Priority: query.Priority,
		Grep:     query.Grep,
		Kernel:   query.Kernel,
		Lines:    int(query.Lines),
	})
	if err != nil {

		h.logger.Warn("log query failed", "query_id", queryID, "error", err)
		return &pb.LogQueryResult{QueryId: query.QueryId, Success: false, Error: err.Error()}, nil
	}

	logs := strings.Join(lines, "\n")

	if len(logs) > 1<<20 {
		logs = logs[len(logs)-(1<<20):]
	}

	h.logger.Info("log query completed", "query_id", queryID, "bytes", len(logs))
	return &pb.LogQueryResult{
		QueryId: query.QueryId,
		Success: true,
		Logs:    logs,
	}, nil
}

func (h *Handler) OnRequestInventory(ctx context.Context, req *pb.RequestInventory) *pb.DeviceInventory {
	return h.CollectInventory(ctx)
}

func (h *Handler) CollectInventory(ctx context.Context) *pb.DeviceInventory {

	tables := h.collectBaselineInventory(ctx)

	oq := h.getOsquery()
	if oq != nil {
		h.supplementWithOsquery(ctx, oq, tables)
	}

	if len(tables) == 0 {
		return nil
	}

	result := make([]*pb.InventoryTable, 0, len(tables))
	for _, t := range tables {
		result = append(result, t)
	}

	h.logger.Info("inventory collected", "tables", len(result), "osquery", oq != nil)
	return &pb.DeviceInventory{Tables: result}
}

func (h *Handler) collectBaselineInventory(ctx context.Context) map[string]*pb.InventoryTable {
	tables := make(map[string]*pb.InventoryTable)

	inv, err := inventory.New(handlerRunner)
	if err != nil {
		h.logger.Debug("baseline inventory unavailable", "error", err)
		return tables
	}

	if sysInfo, err := inv.System(ctx); err == nil {
		tables["system_info"] = &pb.InventoryTable{
			TableName: "system_info",
			Rows: []*pb.OSQueryRow{{Data: map[string]string{
				"hostname":          sysInfo.Hostname,
				"cpu_brand":         sysInfo.CPUModel,
				"cpu_logical_cores": strconv.Itoa(sysInfo.CPUCores),
				"physical_memory":   strconv.FormatInt(sysInfo.MemoryTotalMB*1024*1024, 10),
			}}},
		}
		if sysInfo.KernelVersion != "" {
			tables["kernel_info"] = &pb.InventoryTable{
				TableName: "kernel_info",
				Rows: []*pb.OSQueryRow{{Data: map[string]string{
					"version": sysInfo.KernelVersion,
				}}},
			}
		}
	} else {
		h.logger.Debug("baseline system_info unavailable", "error", err)
	}

	if osInfo, err := inv.OS(); err == nil {
		tables["os_version"] = &pb.InventoryTable{
			TableName: "os_version",
			Rows: []*pb.OSQueryRow{{Data: map[string]string{
				"name":     osInfo.Name,
				"version":  osInfo.Version,
				"platform": osInfo.ID,
				"arch":     osInfo.Arch,
			}}},
		}
	} else {
		h.logger.Debug("baseline os_version unavailable", "error", err)
	}

	if disks, err := inv.Disks(ctx); err == nil {
		var rows []*pb.OSQueryRow
		for _, d := range disks {
			rows = append(rows, &pb.OSQueryRow{Data: map[string]string{
				"name":  d.Device,
				"size":  d.Size,
				"type":  d.Type,
				"label": d.Mount,
			}})
		}
		if len(rows) > 0 {
			tables["block_devices"] = &pb.InventoryTable{
				TableName: "block_devices",
				Rows:      rows,
			}
		}
	} else {
		h.logger.Debug("baseline block_devices unavailable", "error", err)
	}

	if ifaces, err := inv.NetworkInterfaces(ctx); err == nil {
		var detailRows, addrRows []*pb.OSQueryRow
		for _, iface := range ifaces {
			detailRows = append(detailRows, &pb.OSQueryRow{Data: map[string]string{
				"interface": iface.Name,
				"mac":       iface.MAC,
				"type":      "",
			}})
			for _, addr := range iface.Addresses {
				addrRows = append(addrRows, &pb.OSQueryRow{Data: map[string]string{
					"interface": iface.Name,
					"address":   addr,
				}})
			}
		}
		if len(detailRows) > 0 {
			tables["interface_details"] = &pb.InventoryTable{
				TableName: "interface_details",
				Rows:      detailRows,
			}
		}
		if len(addrRows) > 0 {
			tables["interface_addresses"] = &pb.InventoryTable{
				TableName: "interface_addresses",
				Rows:      addrRows,
			}
		}
	} else {
		h.logger.Debug("baseline network interfaces unavailable", "error", err)
	}

	return tables
}

var (
	inventoryCoreTables = []string{
		"system_info",
		"os_version",
		"kernel_info",
		"block_devices",
		"interface_details",
		"interface_addresses",
		"usb_devices",
		"pci_devices",
		"memory_info",
	}
	inventoryPackageTables = []string{
		"deb_packages",
		"rpm_packages",
		"python_packages",
	}
)

func (h *Handler) supplementWithOsquery(ctx context.Context, oq osquery.Querier, baseline map[string]*pb.InventoryTable) {

	coreTables := inventoryCoreTables

	packageTables := inventoryPackageTables

	for _, tableName := range coreTables {
		rows, err := oq.QueryTable(ctx, tableName)
		if err != nil {
			h.logger.Debug("osquery table unavailable", "table", tableName, "error", err)
			continue
		}

		if len(rows) > 0 {
			baseline[tableName] = &pb.InventoryTable{
				TableName: tableName,
				Rows:      osqueryRowsToProto(rows),
			}
		}
	}

	for _, tableName := range packageTables {
		rows, err := oq.QueryTable(ctx, tableName)
		if err != nil {
			continue
		}
		if len(rows) > 0 {
			baseline[tableName] = &pb.InventoryTable{
				TableName: tableName,
				Rows:      osqueryRowsToProto(rows),
			}
		}
	}

}

func queryOsquery(ctx context.Context, oq osquery.Querier, query *pb.OSQuery) (*pb.OSQueryResult, error) {
	var (
		rows []osquery.Row
		err  error
	)
	if query.GetRawSql() != "" {
		rows, err = oq.QuerySQL(ctx, query.GetRawSql())
	} else {
		rows, err = oq.QueryTable(ctx, query.GetTable())
	}
	result := &pb.OSQueryResult{QueryId: query.GetQueryId(), Success: err == nil}
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.Rows = osqueryRowsToProto(rows)
	return result, nil
}

func osqueryRowsToProto(rows []osquery.Row) []*pb.OSQueryRow {
	converted := make([]*pb.OSQueryRow, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, &pb.OSQueryRow{Data: map[string]string(row)})
	}
	return converted
}

func (h *Handler) Executor() *executor.Executor {
	return h.executor
}
