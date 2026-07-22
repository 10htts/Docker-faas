package provider

import "testing"

// TestFunctionContainerLabelsReservedWin is the RT-235 regression: a deployment
// must NOT be able to override the reserved identity labels via its custom
// labels. A caller forging LabelGateway/LabelFunction/etc. would break ownership
// isolation (e.g. make its containers appear owned by another gateway to evade
// or hijack reclaim).
func TestFunctionContainerLabelsReservedWin(t *testing.T) {
	custom := map[string]string{
		LabelGateway:  "victim-network",   // forged owner
		LabelFunction: "other-function",   // forged name
		LabelType:     "not-a-function",   // forged type
		LabelReplica:  "999",              // forged replica
		LabelNetwork:  "some-other-net",   // forged network
		"com.example": "legit-user-label", // a genuine custom label survives
	}

	got := functionContainerLabels(custom, "myfn", "myfn-net", "my-owner", 3)

	if got[LabelGateway] != "my-owner" {
		t.Errorf("LabelGateway must be the real owner, got %q", got[LabelGateway])
	}
	if got[LabelFunction] != "myfn" {
		t.Errorf("LabelFunction must be the real service name, got %q", got[LabelFunction])
	}
	if got[LabelType] != "function" {
		t.Errorf("LabelType must be 'function', got %q", got[LabelType])
	}
	if got[LabelReplica] != "3" {
		t.Errorf("LabelReplica must be the real index, got %q", got[LabelReplica])
	}
	if got[LabelNetwork] != "myfn-net" {
		t.Errorf("LabelNetwork must be the real network, got %q", got[LabelNetwork])
	}
	// A non-reserved custom label must still pass through.
	if got["com.example"] != "legit-user-label" {
		t.Errorf("genuine custom labels must survive, got %q", got["com.example"])
	}
}
