package executor

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestExecuteFlatpak_ValidatesAppIdAndRemote(t *testing.T) {
	e := &Executor{logger: slog.Default(), now: time.Now}
	ctx := context.Background()

	reject := []*pb.FlatpakParams{
		{AppId: &pb.FlatpakAppId{Value: "--system"}},
		{AppId: &pb.FlatpakAppId{Value: "-y"}},
		{AppId: &pb.FlatpakAppId{Value: "a b"}},
		{AppId: &pb.FlatpakAppId{Value: "org.ok.App"}, Remote: "--from=evil"},
		{AppId: &pb.FlatpakAppId{Value: "org.ok.App"}, Remote: "-x"},
		{AppId: &pb.FlatpakAppId{Value: ""}},
	}

	isValidationErr := func(err error) bool {
		if err == nil {
			return false
		}
		m := err.Error()
		return strings.Contains(m, "invalid") || strings.Contains(m, "is required") || strings.Contains(m, "is empty")
	}
	for i, p := range reject {
		_, _, err := e.executeFlatpak(ctx, p, pb.DesiredState_DESIRED_STATE_PRESENT)
		if !isValidationErr(err) {
			t.Errorf("reject case %d (%+v): want a validation error, got %v", i, p, err)
		}
	}

	if _, _, err := e.executeFlatpak(ctx, &pb.FlatpakParams{AppId: &pb.FlatpakAppId{Value: "org.videolan.VLC"}, Remote: "flathub"}, pb.DesiredState_DESIRED_STATE_PRESENT); err != nil {
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "is required") {
			t.Errorf("valid app-id/remote produced a validation error: %v", err)
		}
	}
}
