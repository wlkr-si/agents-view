package theme

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxSize = 16 * 1024

var colorKeys = strings.Fields(`
	accent selection selection_foreground muted
	background dark_background darker_background lighter_background
	foreground dark_foreground light_foreground bright_foreground
	red yellow orange green cyan blue magenta brown
	bright_red bright_yellow bright_green bright_cyan bright_blue bright_magenta`)

var (
	assignment = regexp.MustCompile(`^([a-z_]+)[ \t]*=[ \t]*(?:"([^"]*)"|'([^']*)')[ \t]*(?:#.*)?$`)
	hexColor   = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	border     = regexp.MustCompile(`^(?:rgb\([0-9a-fA-F]{6}\)|rgba\([0-9a-fA-F]{8}\))(?:[ \t]+(?:rgb\([0-9a-fA-F]{6}\)|rgba\([0-9a-fA-F]{8}\)))*(?:[ \t]+[0-9]+deg)?$`)
)

func assignments(raw []byte, set func(key, value string) error) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("expected UTF-8 text")
	}
	seen := make(map[string]bool)
	for i, line := range strings.Split(strings.TrimPrefix(string(raw), "\uFEFF"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := assignment.FindStringSubmatch(line)
		if parts == nil {
			return fmt.Errorf("line %d: expected key = \"value\"", i+1)
		}
		key, value := parts[1], cmp.Or(parts[2], parts[3])
		if seen[key] {
			return fmt.Errorf("line %d: duplicate key %q", i+1, key)
		}
		seen[key] = true
		if err := set(key, value); err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
	}
	return nil
}

func colorsCSS(raw []byte) (string, error) {
	if len(raw) > MaxSize {
		return "", fmt.Errorf("colors.toml exceeds %d KiB", MaxSize/1024)
	}
	values := make(map[string]string)
	err := assignments(raw, func(key, value string) error {
		switch key {
		case "mode":
			if value != "dark" && value != "light" {
				return fmt.Errorf("mode must be dark or light")
			}
		case "hyprland_active_border", "hyprland_inactive_border":
			if !border.MatchString(value) {
				return fmt.Errorf("%s must be a Hyprland border color or gradient", key)
			}
		default:
			// Some official themes also carry colors for Omarchy's icons and tabs.
			if !slices.Contains(colorKeys, key) && key != "icon_mark" &&
				key != "active_border_color" && key != "active_tab_background" {
				return fmt.Errorf("unknown theme key %q", key)
			}
			if !hexColor.MatchString(value) {
				return fmt.Errorf("%s must be a #rrggbb or #rgb color", key)
			}
		}
		values[key] = value
		return nil
	})
	if err != nil {
		return "", err
	}
	if values["mode"] == "" {
		return "", fmt.Errorf("missing theme key %q", "mode")
	}
	values["selection_foreground"] = cmp.Or(values["selection_foreground"], values["foreground"])
	values["orange"] = cmp.Or(values["orange"], values["yellow"])
	if values["brown"] == "" && values["orange"] != "" {
		values["brown"] = darken(values["orange"])
	}
	var css strings.Builder
	fmt.Fprintf(&css, ":root{color-scheme:%s;", values["mode"])
	for _, key := range colorKeys {
		if values[key] == "" {
			return "", fmt.Errorf("missing theme key %q", key)
		}
		fmt.Fprintf(&css, "--%s:%s;", strings.ReplaceAll(key, "_", "-"), values[key])
	}
	css.WriteByte('}')
	return css.String(), nil
}

// Omarchy derives missing brown by mixing orange with black at 50%.
func darken(color string) string {
	hex := color[1:]
	if len(hex) == 3 {
		hex = fmt.Sprintf("%c%c%c%c%c%c", hex[0], hex[0], hex[1], hex[1], hex[2], hex[2])
	}
	var mixed strings.Builder
	mixed.WriteByte('#')
	for i := 0; i < len(hex); i += 2 {
		channel, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		fmt.Fprintf(&mixed, "%02x", (channel+1)/2)
	}
	return mixed.String()
}
