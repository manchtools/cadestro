package devicegroup

import "testing"

func TestInventoryDeviceRejectsMalformedNumericField(t *testing.T) {
	_, err := inventoryDevice("host", []byte(`{"system_info":[{"cpu_physical_cores":"many"}]}`))
	if err == nil {
		t.Fatal("malformed numeric inventory was accepted")
	}
}
