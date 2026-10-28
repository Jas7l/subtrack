package web

import "os"

// Config says where the built client lives.
//
// Fields:
//   - Dir - the folder `npm run build` writes (web/dist); empty turns the
//     client off and the server answers only the API.
type Config struct {
	Dir string
}

// LoadConfig reads WEB_DIR into a Config.
//
// Recognised env vars:
//   - WEB_DIR (default web/dist, relative to the working directory; set it
//     to an empty value to serve the API only)
func LoadConfig() Config {
	dir, ok := os.LookupEnv("WEB_DIR")
	if !ok {
		dir = "web/dist"
	}
	return Config{Dir: dir}
}
