package doctor

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
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

func capturePhysicalCores(ctx context.Context) int {
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer func() {
				_ = file.Close()
			}()
			if count := linuxPhysicalCoreCount(file); count > 0 {
				return count
			}
		}
	}

	if runtime.GOOS == "darwin" {
		value := platformCommand(ctx, "sysctl", "-n", "hw.physicalcpu")
		count, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && count > 0 {
			return count
		}
	}
	return 0
}

func linuxPhysicalCoreCount(input io.Reader) int {
	scanner := bufio.NewScanner(input)
	cores := make(map[string]struct{})
	physicalID := ""
	coreID := ""

	commit := func() {
		if physicalID != "" && coreID != "" {
			cores[physicalID+":"+coreID] = struct{}{}
		}
		physicalID = ""
		coreID = ""
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			commit()
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "physical id":
			physicalID = strings.TrimSpace(value)
		case "core id":
			coreID = strings.TrimSpace(value)
		}
	}
	commit()
	return len(cores)
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
