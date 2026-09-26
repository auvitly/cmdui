//go:build windows

package platform

import "golang.org/x/sys/windows/registry"

func windowsName() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer key.Close()

	product, _, _ := key.GetStringValue("ProductName")
	displayVersion, _, _ := key.GetStringValue("DisplayVersion")
	build, _, _ := key.GetStringValue("CurrentBuildNumber")
	return formatWindowsVersion(product, displayVersion, build)
}
