package internals

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func collectSystemSpecs() Envelope {
	host, _ := os.Hostname()
	info := Envelope{
		"computer_name":      host,
		"operating_system":   runtime.GOOS + " / " + runtime.GOARCH,
		"cpu":                runtime.GOARCH,
		"gpu":                "Not reported",
		"memory":             "Not reported",
		"storage":            []Envelope{},
		"internet":           internetState(),
		"logical_processors": runtime.NumCPU(),
	}
	if runtime.GOOS == "windows" {
		raw := powerShell(`$cpu=(Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty Name); $gpu=((Get-CimInstance Win32_VideoController | ForEach-Object {$_.Name}) -join ' · '); $memory=[math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1GB,1); $storage=@(Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | ForEach-Object {[pscustomobject]@{DeviceID=$_.DeviceID;Size=[double]$_.Size;FreeSpace=[double]$_.FreeSpace}}); [pscustomobject]@{cpu=$cpu;gpu=$gpu;memory_gb=$memory;storage=$storage} | ConvertTo-Json -Depth 5 -Compress`)
		if raw != "" {
			var snapshot map[string]any
			if json.Unmarshal([]byte(raw), &snapshot) == nil {
				if value := strings.TrimSpace(fmt.Sprint(snapshot["cpu"])); value != "" && value != "<nil>" {
					info["cpu"] = value
				}
				if value := strings.TrimSpace(fmt.Sprint(snapshot["gpu"])); value != "" && value != "<nil>" {
					info["gpu"] = value
				}
				if memory := numberFromAny(snapshot["memory_gb"]); memory > 0 {
					info["memory"] = fmt.Sprintf("%.1f GB", memory)
				}
				if disks, ok := snapshot["storage"]; ok {
					info["storage"] = normalizeWindowsDisks(disks)
				}
			}
		}
	} else if runtime.GOOS == "linux" {
		if raw, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(line, "model name") {
					if _, v, ok := strings.Cut(line, ":"); ok {
						info["cpu"] = strings.TrimSpace(v)
						break
					}
				}
			}
		}
	}
	return info
}

func powerShell(script string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	binary := "powershell.exe"
	if _, err := exec.LookPath(binary); err != nil {
		binary = "powershell"
	}
	cmd := exec.CommandContext(ctx, binary, "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func internetState() string {
	for _, address := range []string{"1.1.1.1:443", "8.8.8.8:53"} {
		conn, err := net.DialTimeout("tcp", address, 1200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return "Connected"
		}
	}
	return "Outside connection not detected"
}

func normalizeWindowsDisks(value any) []Envelope {
	list := []map[string]any{}
	switch v := value.(type) {
	case map[string]any:
		list = append(list, v)
	case []any:
		for _, item := range v {
			if row, ok := item.(map[string]any); ok {
				list = append(list, row)
			}
		}
	}
	out := []Envelope{}
	for _, row := range list {
		device := fmt.Sprint(row["DeviceID"])
		size := numberFromAny(row["Size"])
		free := numberFromAny(row["FreeSpace"])
		out = append(out, Envelope{"drive": device, "size_gb": round1(size / 1024 / 1024 / 1024), "free_gb": round1(free / 1024 / 1024 / 1024)})
	}
	return out
}

func numberFromAny(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
func round1(v float64) float64 { n, _ := strconv.ParseFloat(fmt.Sprintf("%.1f", v), 64); return n }
