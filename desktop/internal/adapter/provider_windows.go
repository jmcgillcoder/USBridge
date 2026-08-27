//go:build windows

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type windowsProvider struct{}

func NewProvider() Provider {
	return windowsProvider{}
}

func (windowsProvider) List(ctx context.Context) ([]Adapter, error) {
	command := exec.CommandContext(
		ctx,
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", windowsAdapterScript,
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.Output()
	if err != nil {
		fallback, fallbackErr := listUsingNetPackage()
		if fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("query Windows network adapters: %w", err)
	}
	var values []Adapter
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, fmt.Errorf("decode Windows network adapters: %w", err)
	}
	for index := range values {
		if values[index].ID == "" {
			values[index].ID = strconv.Itoa(values[index].InterfaceIndex)
		}
		values[index].ID = strings.ToLower(strings.Trim(values[index].ID, "{}"))
		values[index].IPv4 = cleanAddresses(values[index].IPv4, false)
		values[index].IPv6 = cleanAddresses(values[index].IPv6, true)
		values[index].Gateways = cleanAddresses(values[index].Gateways, true)
		values[index].DNSServers = cleanAddresses(values[index].DNSServers, true)
	}
	return values, nil
}

func cleanAddresses(values []string, allowIPv6 bool) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		ip := net.ParseIP(stripZone(value))
		if ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
			continue
		}
		if !allowIPv6 && ip.To4() == nil {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func listUsingNetPackage() ([]Adapter, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]Adapter, 0, len(interfaces))
	for _, networkInterface := range interfaces {
		value := Adapter{
			ID:             strconv.Itoa(networkInterface.Index),
			Name:           networkInterface.Name,
			Description:    networkInterface.Name,
			InterfaceIndex: networkInterface.Index,
			Status:         "Disconnected",
			MACAddress:     networkInterface.HardwareAddr.String(),
		}
		if networkInterface.Flags&net.FlagUp != 0 {
			value.Status = "Up"
		}
		addresses, _ := networkInterface.Addrs()
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr != nil || ip == nil {
				continue
			}
			if ip.To4() != nil {
				value.IPv4 = append(value.IPv4, ip.String())
			} else {
				value.IPv6 = append(value.IPv6, ip.String())
			}
		}
		result = append(result, value)
	}
	return result, nil
}

const windowsAdapterScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$allAddresses = @()
$allConfigurations = @()
try {
    $allAddresses = @(Get-NetIPAddress -ErrorAction Stop | Where-Object { $_.AddressState -eq 'Preferred' })
} catch {}
try {
    $allConfigurations = @(Get-NetIPConfiguration -All -ErrorAction Stop)
} catch {}
$result = @(
    Get-NetAdapter -IncludeHidden | ForEach-Object {
        $adapter = $_
        $addresses = @($allAddresses | Where-Object { $_.InterfaceIndex -eq $adapter.ifIndex })
        $configuration = $allConfigurations | Where-Object { $_.InterfaceIndex -eq $adapter.ifIndex } | Select-Object -First 1
        [PSCustomObject]@{
            id = if ($null -ne $adapter.InterfaceGuid) { $adapter.InterfaceGuid.ToString() } else { $adapter.ifIndex.ToString() }
            name = [string]$adapter.Name
            description = [string]$adapter.InterfaceDescription
            interfaceIndex = [int]$adapter.ifIndex
            status = [string]$adapter.Status
            macAddress = [string]$adapter.MacAddress
            linkSpeed = if ($null -ne $adapter.ReceiveLinkSpeed) { [uint64]$adapter.ReceiveLinkSpeed } else { [uint64]0 }
            ipv4 = @($addresses | Where-Object { $_.AddressFamily -eq 'IPv4' } | ForEach-Object { $_.IPAddress })
            ipv6 = @($addresses | Where-Object { $_.AddressFamily -eq 'IPv6' } | ForEach-Object { $_.IPAddress })
            gateways = @(
                $configuration.IPv4DefaultGateway | ForEach-Object { $_.NextHop }
                $configuration.IPv6DefaultGateway | ForEach-Object { $_.NextHop }
            )
            dnsServers = @($configuration.DNSServer.ServerAddresses)
        }
    }
)
ConvertTo-Json -InputObject $result -Depth 4 -Compress
`
