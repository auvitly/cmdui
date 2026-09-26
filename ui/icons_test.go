package ui

import (
	"encoding/xml"
	"io/fs"
	"strings"
	"testing"
)

func TestAppIconVariantsRenderSVG(t *testing.T) {
	for _, key := range []string{
		"go-color", "go-mono", "grpcui-color", "grpcui-mono", "podman-color", "podman-mono",
	} {
		t.Run(key, func(t *testing.T) {
			markup := string(iconGlyph(key))
			if !strings.Contains(markup, `<svg`) && !strings.Contains(markup, `<img`) {
				t.Fatalf("icon %q did not render vector or image markup: %q", key, markup)
			}
		})
	}
}

func TestPodmanIconVariantsUseTechIconsAsset(t *testing.T) {
	colorIcon := string(iconGlyph("podman-color"))
	monoIcon := string(iconGlyph("podman-mono"))
	assetURL := `/static/icons/podman.svg`
	if !strings.Contains(colorIcon, assetURL) || strings.Contains(colorIcon, "podman-mono-mark") {
		t.Fatalf("color Podman icon does not use the TechIcons asset: %q", colorIcon)
	}
	if !strings.Contains(monoIcon, assetURL) || !strings.Contains(monoIcon, "icon-mono") {
		t.Fatalf("monochrome Podman icon does not use its grayscale variant: %q", monoIcon)
	}
}

func TestBrandSVGsAreEmbeddedAndValid(t *testing.T) {
	for _, name := range []string{"go.svg", "grpc.svg", "podman.svg", "run-spinner.svg"} {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(StaticFiles(), "icons/"+name)
			if err != nil {
				t.Fatalf("read embedded SVG: %v", err)
			}
			var icon struct {
				XMLName xml.Name `xml:"svg"`
				ViewBox string   `xml:"viewBox,attr"`
				Paths   []struct {
					Data string `xml:"d,attr"`
				} `xml:"path"`
				Circles []struct {
					Radius string `xml:"r,attr"`
				} `xml:"circle"`
			}
			if err := xml.Unmarshal(data, &icon); err != nil {
				t.Fatalf("parse embedded SVG: %v", err)
			}
			markup := string(data)
			if icon.XMLName.Local != "svg" || icon.ViewBox == "" || (len(icon.Paths) == 0 && len(icon.Circles) == 0) || strings.Count(markup, "<svg") != 1 || strings.Count(markup, "</svg>") != 1 {
				t.Fatalf("embedded SVG is missing its viewBox or vector paths: %#v", icon)
			}
		})
	}
}

func TestGolangIconVariantsUseFontAwesomeMark(t *testing.T) {
	colorIcon := string(iconGlyph("go-color"))
	monoIcon := string(iconGlyph("go-mono"))
	data, err := fs.ReadFile(StaticFiles(), "icons/go.svg")
	if err != nil {
		t.Fatalf("read embedded Go SVG: %v", err)
	}
	if !strings.Contains(string(data), `viewBox="0 0 640 512"`) || !strings.Contains(string(data), `d="M400.1 194.8`) || !strings.Contains(string(data), `fill="#00ADD8"`) {
		t.Fatalf("embedded Go SVG does not contain the Font Awesome mark: %q", data)
	}
	if !strings.Contains(colorIcon, `/static/icons/go.svg`) || strings.Contains(colorIcon, `icon-mono`) {
		t.Fatalf("color Golang icon does not use the static asset: %q", colorIcon)
	}
	if !strings.Contains(monoIcon, `/static/icons/go.svg`) || !strings.Contains(monoIcon, `icon-mono`) {
		t.Fatalf("monochrome Golang icon does not use the local filter variant: %q", monoIcon)
	}
}
