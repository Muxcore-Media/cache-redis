package internal

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key:         "redis_addr",
			Label:       "Redis Address",
			Type:        contracts.SettingTypeString,
			Value:       m.redis,
			Default:     "localhost:6379",
			Description: "host:port for Redis (REDIS_ADDR)",
			Group:       "Redis",
		},
		{
			Key:         "redis_password",
			Label:       "Redis Password",
			Type:        contracts.SettingTypeSecret,
			Value:       modulesdk.MaskSecret(m.password),
			Description: "Optional Redis AUTH password (REDIS_PASSWORD)",
			Group:       "Redis",
		},
		{
			Key:         "redis_db",
			Label:       "Redis DB Index",
			Type:        contracts.SettingTypeInt,
			Value:       strconv.Itoa(m.db),
			Default:     "0",
			Description: "Redis logical database index (REDIS_DB)",
			Group:       "Redis",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "redis_addr", "REDIS_ADDR":
		if value == "" {
			return fmt.Errorf("redis_addr must not be empty")
		}
		return m.setRedis(value, nil, nil)
	case "redis_password", "REDIS_PASSWORD":
		if value == "********" {
			return nil
		}
		pass := value
		return m.setRedis("", &pass, nil)
	case "redis_db", "REDIS_DB":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid redis_db %q (integer >= 0)", value)
		}
		return m.setRedis("", nil, &n)
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

// setRedis updates connection fields and reconnects when Init has completed.
// Empty addr / nil password / nil db keep the current value.
func (m *Module) setRedis(addr string, password *string, db *int) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()

	newAddr := m.redis
	newPass := m.password
	newDB := m.db
	if addr != "" {
		newAddr = addr
	}
	if password != nil {
		newPass = *password
	}
	if db != nil {
		newDB = *db
	}
	if newAddr == m.redis && newPass == m.password && newDB == m.db {
		return nil
	}

	if m.srv == nil {
		m.redis = newAddr
		m.password = newPass
		m.db = newDB
		return nil
	}

	c, err := cache.New(newAddr, newPass, newDB)
	if err != nil {
		return fmt.Errorf("reconnect Redis: %w", err)
	}
	old := m.srv.ReplaceCache(c)
	m.cache = c
	m.redis = newAddr
	m.password = newPass
	m.db = newDB
	if old != nil {
		_ = old.Close()
	}
	return nil
}
