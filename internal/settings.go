package internal

import (
	"fmt"
	"strconv"
	"strings"

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
			Value:       m.redisCfg.Addr,
			Default:     "localhost:6379",
			Description: "host:port for Redis (REDIS_ADDR)",
			Group:       "Redis",
		},
		{
			Key:         "redis_username",
			Label:       "Redis Username",
			Type:        contracts.SettingTypeString,
			Value:       m.redisCfg.Username,
			Description: "ACL username (REDIS_USERNAME)",
			Group:       "Redis",
		},
		{
			Key:         "redis_password",
			Label:       "Redis Password",
			Type:        contracts.SettingTypeSecret,
			Value:       modulesdk.MaskSecret(m.redisCfg.Password),
			Description: "Optional Redis AUTH password (REDIS_PASSWORD)",
			Group:       "Redis",
		},
		{
			Key:         "redis_db",
			Label:       "Redis DB Index",
			Type:        contracts.SettingTypeInt,
			Value:       strconv.Itoa(m.redisCfg.DB),
			Default:     "0",
			Description: "Redis logical database index (REDIS_DB)",
			Group:       "Redis",
		},
		{
			Key:         "cache_key_prefix",
			Label:       "Key Prefix",
			Type:        contracts.SettingTypeString,
			Value:       m.redisCfg.KeyPrefix,
			Description: "Prefix for all keys/channels (CACHE_KEY_PREFIX); required on shared Redis",
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
		return m.applyRedis(value, nil, nil, nil, nil)
	case "redis_username", "REDIS_USERNAME":
		user := value
		return m.applyRedis("", nil, &user, nil, nil)
	case "redis_password", "REDIS_PASSWORD":
		if value == "********" {
			return nil
		}
		pass := value
		return m.applyRedis("", &pass, nil, nil, nil)
	case "redis_db", "REDIS_DB":
		n, err := parseDB(value)
		if err != nil {
			return err
		}
		return m.applyRedis("", nil, nil, &n, nil)
	case "cache_key_prefix", "CACHE_KEY_PREFIX":
		prefix := value
		return m.applyRedis("", nil, nil, nil, &prefix)
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) applyRedis(addr string, password *string, username *string, db *int, prefix *string) error {
	m.cfgMu.Lock()
	newCfg := mergeRedisConfig(m.redisCfg, addr, password, username, db, prefix)
	if redisConfigEqual(newCfg, m.redisCfg) {
		m.cfgMu.Unlock()
		return nil
	}
	if m.srv == nil {
		m.redisCfg = newCfg
		m.cfgMu.Unlock()
		return nil
	}
	m.cfgMu.Unlock()
	return m.reconnect(newCfg)
}
