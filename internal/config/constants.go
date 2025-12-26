package config

import (
	_ "embed"
)

// レイヤー名定数（後方互換用）
const (
	LayerDefaults       = "defaults"
	LayerUser           = "user"
	LayerProject        = "project"
	LayerEnv            = "env"
	LayerCredentials    = "credentials"
	LayerParameterStore = "parameter_store"
	LayerArgs           = "args"
)

//go:embed defaults.yaml
var defaultConfigYAML []byte
