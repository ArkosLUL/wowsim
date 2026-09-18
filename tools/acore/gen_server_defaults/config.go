package main

import (
	"fmt"
	"strconv"
	"strings"
)

// config answers GetOption the way worldserver does (Config.cpp GetValueDefault): the AC_ environment
// variable first, then the conf files, then the caller's default. A value that doesn't parse also
// gets the default.
type config struct {
	conf map[string]string // conf key -> value
	env  map[string]string // AC_ variable -> value
}

func newConfig() *config {
	return &config{conf: map[string]string{}, env: map[string]string{}}
}

// addConf loads a conf file the way Config.cpp ParseFile does: trimmed lines, # and [ lines skipped,
// split at the first =, every " dropped from the value, and the first of duplicate keys kept.
func (c *config) addConf(src string) {
	seen := map[string]bool{}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if seen[key] {
			continue
		}
		seen[key] = true
		c.conf[key] = strings.ReplaceAll(strings.TrimSpace(value), `"`, "")
	}
}

// addEnvFile loads a docker compose env_file: KEY=value, or the YAML style KEY: "value" the override
// files use. Later files win, as in compose's env_file list. No variable expansion.
func (c *config) addEnvFile(name, src string) error {
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		sep := strings.IndexAny(line, "=:")
		if sep <= 0 {
			return fmt.Errorf("%s:%d: expected KEY=value or KEY: value", name, i+1)
		}
		key := strings.TrimSpace(line[:sep])
		value := strings.TrimSpace(line[sep+1:])
		if value != "" && (value[0] == '"' || value[0] == '\'') {
			end := strings.IndexByte(value[1:], value[0])
			if end < 0 {
				return fmt.Errorf("%s:%d: unterminated quote", name, i+1)
			}
			value = value[1 : end+1]
		} else if j := strings.Index(value, " #"); j >= 0 {
			value = strings.TrimSpace(value[:j])
		}
		c.env[key] = value
	}
	return nil
}

// envVarName ports Config.cpp's IniKeyToEnvVarKey, quirks included:
// DungeonScale.StatModifierRaid25M.Boss.Health -> AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_25_M_BOSS_HEALTH.
func envVarName(key string) string {
	isUpper := func(c byte) bool { return c >= 'A' && c <= 'Z' }
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	toUpper := func(c byte) byte {
		if c >= 'a' && c <= 'z' {
			return c - 'a' + 'A'
		}
		return c
	}

	var b strings.Builder
	b.WriteString("AC_")
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == ' ' || c == '.' || c == '-' {
			b.WriteByte('_')
			continue
		}
		b.WriteByte(toUpper(c))
		if i == len(key)-1 {
			continue
		}
		next := key[i+1]
		if (!isUpper(c) && isUpper(next)) || isDigit(c) != isDigit(next) {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func (c *config) raw(key string) (string, bool) {
	if v, ok := c.env[envVarName(key)]; ok {
		return v, true
	}
	v, ok := c.conf[key]
	return v, ok
}

// parseFloat32 matches StringTo<float> on Linux: std::stold over the whole string, then a cast.
func parseFloat32(s string) (float32, bool) {
	v, err := strconv.ParseFloat(strings.TrimLeft(s, " \t\n\v\f\r"), 32)
	return float32(v), err == nil
}

// optionalFloat is the value only when the key is set and parses.
func (c *config) optionalFloat(key string) (float32, bool) {
	s, ok := c.raw(key)
	if !ok {
		return 0, false
	}
	return parseFloat32(s)
}

func (c *config) float(key string, def float32) float32 {
	if v, ok := c.optionalFloat(key); ok {
		return v
	}
	return def
}

// bool takes what StringTo<bool> does: 1, y, on, yes, true and 0, n, off, no, false, any case.
func (c *config) bool(key string, def bool) bool {
	s, ok := c.raw(key)
	if !ok {
		return def
	}
	switch strings.ToLower(s) {
	case "1", "y", "on", "yes", "true":
		return true
	case "0", "n", "off", "no", "false":
		return false
	}
	return def
}

// uint32 is std::from_chars base 10: digits only, no sign or spaces.
func (c *config) uint32(key string, def uint32) uint32 {
	s, ok := c.raw(key)
	if !ok {
		return def
	}
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return def
	}
	return uint32(v)
}

func (c *config) string(key, def string) string {
	if s, ok := c.raw(key); ok {
		return s
	}
	return def
}
