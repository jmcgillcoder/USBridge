//go:build windows

package exclusivenet

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"
)

func runHelperIfRequested(arguments []string) (bool, int) {
	requested := false
	for _, argument := range arguments {
		if argument == "--usbridge-exclusive-helper" {
			requested = true
			break
		}
	}
	if !requested {
		return false, 0
	}

	flags := flag.NewFlagSet("usbridge-exclusive-helper", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	helperMode := flags.Bool("usbridge-exclusive-helper", false, "")
	controlAddress := flags.String("exclusive-control", "", "")
	token := flags.String("exclusive-token", "", "")
	if err := flags.Parse(arguments); err != nil || !*helperMode {
		return true, 2
	}
	if err := validateHelperArguments(*controlAddress, *token); err != nil {
		return true, 2
	}
	if err := runElevatedHelper(*controlAddress, *token); err != nil {
		return true, 1
	}
	return true, 0
}

func validateHelperArguments(controlAddress, token string) error {
	address, err := netip.ParseAddrPort(controlAddress)
	if err != nil || !address.Addr().Is4() || !address.Addr().IsLoopback() || address.Port() == 0 {
		return errors.New("invalid helper control address")
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return errors.New("invalid helper token")
	}
	return nil
}

func runElevatedHelper(controlAddress, token string) error {
	connection, err := net.DialTimeout("tcp4", controlAddress, 15*time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	if tcpConnection, ok := connection.(*net.TCPConn); ok {
		_ = tcpConnection.SetKeepAlive(true)
		_ = tcpConnection.SetKeepAlivePeriod(10 * time.Second)
	}

	encoder := json.NewEncoder(connection)
	decoder := json.NewDecoder(connection)
	if err := encoder.Encode(helperHello{Protocol: helperProtocolVersion, Token: token}); err != nil {
		return err
	}

	executable, err := os.Executable()
	if err != nil {
		return err
	}
	var activePolicy *wfpPolicy
	defer func() {
		if activePolicy != nil {
			activePolicy.Close()
		}
	}()

	for {
		var request helperRequest
		if err := decoder.Decode(&request); err != nil {
			return err
		}

		response := helperResponse{ID: request.ID}
		switch request.Command {
		case helperCommandApply:
			if request.InterfaceIndex <= 0 {
				response.Error = "无效的 Windows 网卡"
				break
			}
			policy, policyErr := installWFPPolicy(request.InterfaceIndex, executable)
			if policyErr != nil {
				response.Error = policyErr.Error()
				if errors.Is(policyErr, ErrAdministratorRequired) {
					response.ErrorCode = helperErrorAdministratorRequired
				}
				if activePolicy != nil {
					response.Active = true
					response.InterfaceIndex = activePolicy.interfaceIndex
				}
				break
			}
			previous := activePolicy
			activePolicy = policy
			if previous != nil {
				previous.Close()
			}
			response.OK = true
			response.Active = true
			response.InterfaceIndex = request.InterfaceIndex

		case helperCommandDisable:
			if activePolicy != nil {
				activePolicy.Close()
				activePolicy = nil
			}
			response.OK = true

		case helperCommandPing:
			if activePolicy != nil {
				if policyErr := activePolicy.Check(); policyErr != nil {
					activePolicy.Close()
					activePolicy = nil
					response.Error = "Windows 网络保护规则已失效：" + policyErr.Error()
					break
				}
			}
			response.OK = true
			if activePolicy != nil {
				response.Active = true
				response.InterfaceIndex = activePolicy.interfaceIndex
			}

		case helperCommandExit:
			if activePolicy != nil {
				activePolicy.Close()
				activePolicy = nil
			}
			response.OK = true
			if err := encoder.Encode(response); err != nil {
				return err
			}
			return nil

		default:
			response.Error = "不支持的严格代理模式操作"
		}

		if response.Error == "" && !response.OK {
			response.Error = "严格代理模式操作未完成"
		}
		response.Error = strings.TrimSpace(response.Error)
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
}
