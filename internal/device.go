package internal

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// 官网 completions 会带 x-device-id（形如 uid_ + 16 位小写字母数字），
// 浏览器侧持久化；这里同样写到 data/device_id.txt，进程间复用。
const (
	deviceIDPrefix   = "uid_"
	deviceIDRandLen  = 16
	deviceIDAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
)

var (
	deviceIDOnce sync.Once
	deviceID     string
)

func GetDeviceID() string {
	deviceIDOnce.Do(func() {
		path := filepath.Join("data", "device_id.txt")
		if b, err := os.ReadFile(path); err == nil {
			id := strings.TrimSpace(string(b))
			if isValidDeviceID(id) {
				deviceID = id
				return
			}
		}
		id := generateDeviceID()
		deviceID = id
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, []byte(id+"\n"), 0644)
	})
	return deviceID
}

func isValidDeviceID(id string) bool {
	if !strings.HasPrefix(id, deviceIDPrefix) || len(id) != len(deviceIDPrefix)+deviceIDRandLen {
		return false
	}
	for _, c := range id[len(deviceIDPrefix):] {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func generateDeviceID() string {
	raw := make([]byte, deviceIDRandLen)
	if _, err := rand.Read(raw); err != nil {
		return deviceIDPrefix + strings.Repeat("0", deviceIDRandLen)
	}
	out := make([]byte, deviceIDRandLen)
	for i, v := range raw {
		out[i] = deviceIDAlphabet[int(v)%len(deviceIDAlphabet)]
	}
	return deviceIDPrefix + string(out)
}
