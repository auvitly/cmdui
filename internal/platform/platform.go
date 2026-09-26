package platform

import (
	"bufio"
	"encoding/xml"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func Current() string {
	switch runtime.GOOS {
	case "linux":
		return linuxName("/etc/os-release")
	case "windows":
		return windowsName()
	case "darwin":
		return macOSName("/System/Library/CoreServices/SystemVersion.plist")
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "dragonfly":
		return "DragonFly BSD"
	case "solaris":
		return "Solaris"
	default:
		if runtime.GOOS == "" {
			return "Неизвестная платформа"
		}
		return runtime.GOOS
	}
}

func formatWindowsVersion(product, displayVersion, build string) string {
	product = strings.TrimSpace(product)
	if product == "" {
		product = "Windows"
	}
	buildNumber, _ := strconv.Atoi(strings.TrimSpace(build))
	if buildNumber >= 22000 && strings.HasPrefix(strings.ToLower(product), "windows 10") {
		product = "Windows 11" + strings.TrimPrefix(product, "Windows 10")
	}
	version := strings.TrimSpace(displayVersion)
	if build != "" {
		if version != "" {
			version += " · "
		}
		version += "Сборка " + strings.TrimSpace(build)
	}
	if version == "" {
		return product
	}
	return product + " · " + version
}

func macOSName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "macOS"
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var pendingKey string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var key string
			if decoder.DecodeElement(&key, &start) == nil {
				pendingKey = key
			}
		case "string":
			var value string
			if decoder.DecodeElement(&value, &start) == nil && pendingKey == "ProductVersion" && value != "" {
				return "macOS " + value
			}
			pendingKey = ""
		}
	}
	return "macOS"
}

func linuxName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "Linux"
	}
	values := parseOSRelease(data)
	distro := values["PRETTY_NAME"]
	if distro == "" {
		distro = values["NAME"]
	}
	if distro == "" {
		distro = values["ID"]
	}
	if distro == "" {
		return "Linux"
	}
	if strings.Contains(strings.ToLower(distro), "linux") {
		return distro
	}
	return "Linux · " + distro
}

func parseOSRelease(data []byte) map[string]string {
	values := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if key == "" {
			continue
		}
		if len(rawValue) >= 2 && rawValue[0] == '"' && rawValue[len(rawValue)-1] == '"' {
			if unquoted, err := strconv.Unquote(rawValue); err == nil {
				rawValue = unquoted
			} else {
				rawValue = rawValue[1 : len(rawValue)-1]
			}
		} else if len(rawValue) >= 2 && rawValue[0] == '\'' && rawValue[len(rawValue)-1] == '\'' {
			rawValue = rawValue[1 : len(rawValue)-1]
		}
		values[key] = rawValue
	}
	return values
}
