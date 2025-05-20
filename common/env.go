package common

import (
	"fmt"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
	"os"
	"strings"
)

var PORT string
var RELAY_URL string
var RELAY_NAME string
var RELAY_ICON string
var RELAY_ADMIN string
var RELAY_SECRET string
var RELAY_SELF string
var RELAY_DESCRIPTION string
var RELAY_CLAIMS []string
var AUTH_BACKEND string
var AUTH_WHITELIST []string
var AUTH_RESTRICT_USER bool
var AUTH_RESTRICT_AUTHOR bool
var GENERATE_CLAIMS bool
var DATA_DIR string

func SetupEnvironment() {
	var env = make(map[string]string)

	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		env[parts[0]] = parts[1]
	}

	getEnv := func(k string, fallback ...string) (v string) {
		v = env[k]

		if v == "" && len(fallback) > 0 {
			v = fallback[0]
		}

		return v
	}

	PORT = getEnv("PORT", "3334")
	RELAY_URL = getEnv("RELAY_URL", "localhost:3334")
	RELAY_NAME = getEnv("RELAY_NAME", "Frith")
	RELAY_ICON = getEnv("RELAY_ICON", "https://hbr.coracle.social/fd73de98153b615f516d316d663b413205fd2e6e53d2c6064030ab57a7685bbd.jpg")
	RELAY_ADMIN = getEnv("RELAY_ADMIN", "")
	RELAY_SECRET = getEnv("RELAY_SECRET", nostr.GeneratePrivateKey())
	RELAY_SELF, _ = nostr.GetPublicKey(RELAY_SECRET)
	RELAY_DESCRIPTION = getEnv("RELAY_DESCRIPTION", "A nostr relay for hosting groups.")
	RELAY_CLAIMS = split(getEnv("RELAY_CLAIMS", ""), ",")
	AUTH_BACKEND = getEnv("AUTH_BACKEND", "")
	AUTH_WHITELIST = split(getEnv("AUTH_WHITELIST", ""), ",")
	AUTH_RESTRICT_USER = getEnv("AUTH_RESTRICT_USER", "true") == "true"
	AUTH_RESTRICT_AUTHOR = getEnv("AUTH_RESTRICT_AUTHOR", "false") == "true"
	GENERATE_CLAIMS = getEnv("GENERATE_CLAIMS", "false") == "true"
	DATA_DIR = getEnv("DATA_DIR", "./data")
}

func GetDataDir(dir string) string {
	return fmt.Sprintf("%s/%s", DATA_DIR, dir)
}
