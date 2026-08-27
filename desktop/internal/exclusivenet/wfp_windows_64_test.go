//go:build windows && (amd64 || arm64)

package exclusivenet

import (
	"testing"
	"unsafe"
)

func TestWFPStructureLayout(t *testing.T) {
	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{name: "FWP_BYTE_BLOB", got: unsafe.Sizeof(fwpByteBlob{}), want: 16},
		{name: "FWP_VALUE0", got: unsafe.Sizeof(fwpValue0{}), want: 16},
		{name: "FWPM_DISPLAY_DATA0", got: unsafe.Sizeof(fwpmDisplayData0{}), want: 16},
		{name: "FWPM_ACTION0", got: unsafe.Sizeof(fwpmAction0{}), want: 20},
		{name: "FWPM_FILTER_CONDITION0", got: unsafe.Sizeof(fwpmFilterCondition0{}), want: 40},
		{name: "FWPM_FILTER0", got: unsafe.Sizeof(fwpmFilter0{}), want: 200},
		{name: "FWPM_SUBLAYER0", got: unsafe.Sizeof(fwpmSublayer0{}), want: 72},
		{name: "FWPM_SESSION0", got: unsafe.Sizeof(fwpmSession0{}), want: 72},
	}
	for _, test := range tests {
		if test.got != test.want {
			t.Errorf("%s size = %d, want %d", test.name, test.got, test.want)
		}
	}
}

func TestWFPProceduresAreAvailable(t *testing.T) {
	procedures := map[string]interface{ Find() error }{
		"FwpmEngineOpen0":           procFwpmEngineOpen0,
		"FwpmEngineClose0":          procFwpmEngineClose0,
		"FwpmTransactionBegin0":     procFwpmTransactionBegin,
		"FwpmTransactionCommit0":    procFwpmTransactionCommit,
		"FwpmTransactionAbort0":     procFwpmTransactionAbort,
		"FwpmSubLayerAdd0":          procFwpmSubLayerAdd0,
		"FwpmFilterAdd0":            procFwpmFilterAdd0,
		"FwpmFilterGetById0":        procFwpmFilterGetByID0,
		"FwpmGetAppIdFromFileName0": procFwpmGetAppID,
		"FwpmFreeMemory0":           procFwpmFreeMemory0,
	}
	for name, procedure := range procedures {
		if err := procedure.Find(); err != nil {
			t.Errorf("%s is unavailable: %v", name, err)
		}
	}
}
