package doctor

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const unavailablePlatformValue = "unknown"

func captureKernelVersion(ctx context.Context) string {
	if runtime.GOOS == "linux" {
		if value := readTrimmedFile("/proc/sys/kernel/osrelease"); value != "" {
			return value
		}
	}

	switch runtime.GOOS {
	case "darwin", "freebsd", "openbsd", "netbsd":
		return platformCommand(ctx, "uname", "-r")
	case "windows":
		return platformCommand(ctx, "cmd", "/c", "ver")
	default:
		return unavailablePlatformValue
	}
}

func captureCPUModel(ctx context.Context) string {
	if runtime.GOOS == "linux" {
		if value := linuxCPUModel(); value != "" {
			return value
		}
	}

	switch runtime.GOOS {
	case "darwin":
		return platformCommand(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
	case "freebsd":
		return platformCommand(ctx, "sysctl", "-n", "hw.model")
	case "windows":
		if value := strings.TrimSpace(os.Getenv("PROCESSOR_IDENTIFIER")); value != "" {
			return value
		}
	}
	return unavailablePlatformValue
}

func linuxCPUModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	var fallback string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "model name":
			return value
		case "Hardware", "Processor":
			if fallback == "" && value != "" {
				fallback = value
			}
		}
	}
	return fallback
}

func readTrimmedFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func platformCommand(ctx context.Context, command string, args ...string) string {
	probeContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	output, err := exec.CommandContext(probeContext, command, args...).CombinedOutput()
	if err != nil {
		return unavailablePlatformValue
	}
	if value := firstLine(string(output)); value != "" {
		return value
	}
	return unavailablePlatformValue
}
