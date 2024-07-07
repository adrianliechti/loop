package docker

type ConfigFile struct {
	AuthConfigs map[string]AuthConfig `json:"auths"`
}

type AuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	Auth string `json:"auth,omitempty"`
}
