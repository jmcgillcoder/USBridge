//go:build windows && (amd64 || arm64)

package exclusivenet

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	rpcAuthnWinNT = 10

	fwpmSessionFlagDynamic         = 0x00000001
	fwpmFilterFlagClearActionRight = 0x00000008

	fwpMatchEqual = 0

	fwpEmpty        = 0
	fwpUint8        = 1
	fwpUint16       = 2
	fwpUint64       = 4
	fwpByteBlobType = 12

	fwpActionBlock  = 0x00001001
	fwpActionPermit = 0x00001002
)

var (
	fwpmLayerALEAuthConnectV4 = windows.GUID{
		Data1: 0xc38d57d1, Data2: 0x05a7, Data3: 0x4c33,
		Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82},
	}
	fwpmLayerALEAuthConnectV6 = windows.GUID{
		Data1: 0x4a72393b, Data2: 0x319f, Data3: 0x44bc,
		Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4},
	}
	fwpmConditionALEAppID = windows.GUID{
		Data1: 0xd78e1e87, Data2: 0x8644, Data3: 0x4ea5,
		Data4: [8]byte{0x94, 0x37, 0xd8, 0x09, 0xec, 0xef, 0xc9, 0x71},
	}
	fwpmConditionIPLocalInterface = windows.GUID{
		Data1: 0x4cd62a49, Data2: 0x59c3, Data3: 0x4969,
		Data4: [8]byte{0xb7, 0xf3, 0xbd, 0xa5, 0xd3, 0x28, 0x90, 0xa4},
	}
	fwpmConditionIPProtocol = windows.GUID{
		Data1: 0x3971ef2b, Data2: 0x623e, Data3: 0x4f9a,
		Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7},
	}
	fwpmConditionIPLocalPort = windows.GUID{
		Data1: 0x0c1ba1af, Data2: 0x5765, Data3: 0x453f,
		Data4: [8]byte{0xaf, 0x22, 0xa8, 0xf7, 0x91, 0xac, 0x77, 0x5b},
	}
	fwpmConditionIPRemotePort = windows.GUID{
		Data1: 0xc35a604d, Data2: 0xd22b, Data3: 0x4e1a,
		Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b},
	}
)

var (
	modFwpuclnt               = windows.NewLazySystemDLL("fwpuclnt.dll")
	procFwpmEngineOpen0       = modFwpuclnt.NewProc("FwpmEngineOpen0")
	procFwpmEngineClose0      = modFwpuclnt.NewProc("FwpmEngineClose0")
	procFwpmTransactionBegin  = modFwpuclnt.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit = modFwpuclnt.NewProc("FwpmTransactionCommit0")
	procFwpmTransactionAbort  = modFwpuclnt.NewProc("FwpmTransactionAbort0")
	procFwpmSubLayerAdd0      = modFwpuclnt.NewProc("FwpmSubLayerAdd0")
	procFwpmFilterAdd0        = modFwpuclnt.NewProc("FwpmFilterAdd0")
	procFwpmFilterGetByID0    = modFwpuclnt.NewProc("FwpmFilterGetById0")
	procFwpmGetAppID          = modFwpuclnt.NewProc("FwpmGetAppIdFromFileName0")
	procFwpmFreeMemory0       = modFwpuclnt.NewProc("FwpmFreeMemory0")
)

type fwpByteBlob struct {
	size uint32
	_    uint32
	data *byte
}

type fwpValue0 struct {
	type_ uint32
	_     uint32
	value uintptr
}

type fwpmDisplayData0 struct {
	name        *uint16
	description *uint16
}

type fwpmAction0 struct {
	type_ uint32
	guid  windows.GUID
}

type fwpmFilterCondition0 struct {
	fieldKey       windows.GUID
	matchType      uint32
	_              uint32
	conditionValue fwpValue0
}

type fwpmFilter0 struct {
	filterKey           windows.GUID
	displayData         fwpmDisplayData0
	flags               uint32
	_                   uint32
	providerKey         *windows.GUID
	providerData        fwpByteBlob
	layerKey            windows.GUID
	subLayerKey         windows.GUID
	weight              fwpValue0
	numFilterConditions uint32
	_                   uint32
	filterCondition     *fwpmFilterCondition0
	action              fwpmAction0
	_                   [4]byte
	providerContextKey  windows.GUID
	reserved            *windows.GUID
	filterID            uint64
	effectiveWeight     fwpValue0
}

type fwpmSublayer0 struct {
	subLayerKey  windows.GUID
	displayData  fwpmDisplayData0
	flags        uint32
	_            uint32
	providerKey  *windows.GUID
	providerData fwpByteBlob
	weight       uint16
	_            [6]byte
}

type fwpmSession0 struct {
	sessionKey           windows.GUID
	displayData          fwpmDisplayData0
	flags                uint32
	txnWaitTimeoutInMSec uint32
	processID            uint32
	_                    uint32
	sid                  *windows.SID
	username             *uint16
	kernelMode           uint8
	_                    [7]byte
}

type wfpPolicy struct {
	engine         uintptr
	interfaceIndex int
	filterIDs      []uint64
}

func installWFPPolicy(interfaceIndex int, executable string) (*wfpPolicy, error) {
	if interfaceIndex <= 0 {
		return nil, errors.New("无效的 Windows 网卡")
	}
	row := windows.MibIfRow2{InterfaceIndex: uint32(interfaceIndex)}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics, &row); err != nil {
		return nil, fmt.Errorf("读取 Windows 网卡失败：%w", err)
	}
	if row.InterfaceLuid == 0 {
		return nil, errors.New("Windows 网卡没有可用的接口标识")
	}

	appID, err := getWFPAppID(executable)
	if err != nil {
		return nil, err
	}
	defer freeWFPMemory(unsafe.Pointer(&appID))

	engine, err := openWFPEngine()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			closeWFPEngine(engine)
		}
	}()

	if err := callWFP("开始规则事务", procFwpmTransactionBegin, engine, 0); err != nil {
		return nil, err
	}
	transactionOpen := true
	defer func() {
		if transactionOpen {
			_, _, _ = procFwpmTransactionAbort.Call(engine)
		}
	}()

	sublayerKey, err := windows.GenerateGUID()
	if err != nil {
		return nil, fmt.Errorf("创建独占规则标识失败：%w", err)
	}
	if err := addWFPSublayer(engine, sublayerKey); err != nil {
		return nil, err
	}
	filterIDs := make([]uint64, 0, len(buildPolicyRules()))
	for _, rule := range buildPolicyRules() {
		filterID, addErr := addWFPPolicyRule(engine, sublayerKey, row.InterfaceLuid, appID, rule)
		if addErr != nil {
			return nil, addErr
		}
		filterIDs = append(filterIDs, filterID)
	}
	if err := callWFP("提交网络保护规则", procFwpmTransactionCommit, engine); err != nil {
		return nil, err
	}
	transactionOpen = false
	committed = true
	return &wfpPolicy{engine: engine, interfaceIndex: interfaceIndex, filterIDs: filterIDs}, nil
}

func (p *wfpPolicy) Check() error {
	if p == nil || p.engine == 0 || len(p.filterIDs) == 0 {
		return errors.New("Windows 网络保护会话已关闭")
	}
	for _, filterID := range p.filterIDs {
		if err := checkWFPFilter(p.engine, filterID); err != nil {
			return err
		}
	}
	return nil
}

func (p *wfpPolicy) Close() {
	if p == nil || p.engine == 0 {
		return
	}
	closeWFPEngine(p.engine)
	p.engine = 0
}

func openWFPEngine() (uintptr, error) {
	name, _ := windows.UTF16PtrFromString("USBridge 严格代理模式")
	description, _ := windows.UTF16PtrFromString("仅允许 USBridge 使用所选手机 USB 网卡")
	session := fwpmSession0{
		displayData: fwpmDisplayData0{name: name, description: description},
		flags:       fwpmSessionFlagDynamic,
	}
	var engine uintptr
	err := callWFP(
		"打开 Windows 网络保护服务",
		procFwpmEngineOpen0,
		0,
		rpcAuthnWinNT,
		0,
		uintptr(unsafe.Pointer(&session)),
		uintptr(unsafe.Pointer(&engine)),
	)
	runtime.KeepAlive(session)
	if err != nil {
		return 0, err
	}
	return engine, nil
}

func closeWFPEngine(engine uintptr) {
	if engine != 0 {
		_, _, _ = procFwpmEngineClose0.Call(engine)
	}
}

func getWFPAppID(executable string) (*fwpByteBlob, error) {
	path, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return nil, fmt.Errorf("读取程序路径失败：%w", err)
	}
	var appID *fwpByteBlob
	if err := callWFP(
		"读取 USBridge 程序标识",
		procFwpmGetAppID,
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&appID)),
	); err != nil {
		return nil, err
	}
	if appID == nil || appID.size == 0 || appID.data == nil {
		freeWFPMemory(unsafe.Pointer(&appID))
		return nil, errors.New("Windows 没有返回 USBridge 程序标识")
	}
	return appID, nil
}

func freeWFPMemory(pointer unsafe.Pointer) {
	if pointer != nil {
		_, _, _ = procFwpmFreeMemory0.Call(uintptr(pointer))
	}
}

func addWFPSublayer(engine uintptr, key windows.GUID) error {
	name, _ := windows.UTF16PtrFromString("USBridge 手机 USB 网卡保护")
	description, _ := windows.UTF16PtrFromString("阻止其他应用直接使用手机 USB 网络")
	sublayer := fwpmSublayer0{
		subLayerKey: key,
		displayData: fwpmDisplayData0{name: name, description: description},
		weight:      ^uint16(0),
	}
	err := callWFP(
		"创建网络保护规则组",
		procFwpmSubLayerAdd0,
		engine,
		uintptr(unsafe.Pointer(&sublayer)),
		0,
	)
	runtime.KeepAlive(sublayer)
	return err
}

func addWFPPolicyRule(engine uintptr, sublayer windows.GUID, luid uint64, appID *fwpByteBlob, rule policyRule) (uint64, error) {
	luidValue := new(uint64)
	*luidValue = luid
	var pinner runtime.Pinner
	pinner.Pin(luidValue)
	defer pinner.Unpin()

	conditions := make([]fwpmFilterCondition0, len(rule.conditions))
	for index, condition := range rule.conditions {
		value := fwpValue0{}
		field := windows.GUID{}
		switch condition.kind {
		case conditionInterface:
			field = fwpmConditionIPLocalInterface
			value.type_ = fwpUint64
			value.value = uintptr(unsafe.Pointer(luidValue))
		case conditionAppID:
			field = fwpmConditionALEAppID
			value.type_ = fwpByteBlobType
			value.value = uintptr(unsafe.Pointer(appID))
		case conditionProtocol:
			field = fwpmConditionIPProtocol
			value.type_ = fwpUint8
			value.value = uintptr(uint8(condition.value))
		case conditionLocalPort, conditionICMPType:
			field = fwpmConditionIPLocalPort
			value.type_ = fwpUint16
			value.value = uintptr(condition.value)
		case conditionRemotePort, conditionICMPCode:
			field = fwpmConditionIPRemotePort
			value.type_ = fwpUint16
			value.value = uintptr(condition.value)
		default:
			return 0, fmt.Errorf("独占规则 %q 包含未知条件", rule.name)
		}
		conditions[index] = fwpmFilterCondition0{
			fieldKey:       field,
			matchType:      fwpMatchEqual,
			conditionValue: value,
		}
	}

	layer := fwpmLayerALEAuthConnectV4
	if rule.family == policyFamilyIPv6 {
		layer = fwpmLayerALEAuthConnectV6
	}
	action := uint32(fwpActionPermit)
	if rule.action == policyActionBlock {
		action = fwpActionBlock
	}
	name, _ := windows.UTF16PtrFromString(rule.name)
	description, _ := windows.UTF16PtrFromString("USBridge exclusive phone USB interface policy")
	filter := fwpmFilter0{
		displayData:         fwpmDisplayData0{name: name, description: description},
		layerKey:            layer,
		subLayerKey:         sublayer,
		weight:              fwpValue0{type_: fwpUint8, value: uintptr(rule.weight)},
		numFilterConditions: uint32(len(conditions)),
		filterCondition:     &conditions[0],
		action:              fwpmAction0{type_: action},
	}
	if rule.hardAction {
		filter.flags |= fwpmFilterFlagClearActionRight
	}
	var filterID uint64
	err := callWFP(
		"安装网络保护规则",
		procFwpmFilterAdd0,
		engine,
		uintptr(unsafe.Pointer(&filter)),
		0,
		uintptr(unsafe.Pointer(&filterID)),
	)
	runtime.KeepAlive(luidValue)
	runtime.KeepAlive(appID)
	runtime.KeepAlive(conditions)
	runtime.KeepAlive(filter)
	return filterID, err
}

func checkWFPFilter(engine uintptr, filterID uint64) error {
	if filterID == 0 {
		return errors.New("Windows 网络保护规则标识无效")
	}
	var filter *fwpmFilter0
	if err := callWFP(
		"检查网络保护规则",
		procFwpmFilterGetByID0,
		engine,
		uintptr(filterID),
		uintptr(unsafe.Pointer(&filter)),
	); err != nil {
		return err
	}
	defer freeWFPMemory(unsafe.Pointer(&filter))
	if filter == nil {
		return errors.New("Windows 网络保护规则不存在")
	}
	return nil
}

type wfpAPIError struct {
	operation string
	code      uint32
}

func (e *wfpAPIError) Error() string {
	return fmt.Sprintf("%s失败（Windows 错误 0x%08x）", e.operation, e.code)
}

func (e *wfpAPIError) Is(target error) bool {
	return target == ErrAdministratorRequired && (e.code == uint32(windows.ERROR_ACCESS_DENIED) || e.code == 0x80070005)
}

func callWFP(operation string, procedure *windows.LazyProc, arguments ...uintptr) error {
	result, _, _ := procedure.Call(arguments...)
	if uint32(result) == 0 {
		return nil
	}
	return &wfpAPIError{operation: operation, code: uint32(result)}
}
