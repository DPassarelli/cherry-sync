package canary

import (
        "os"
        "strings"
)

func splitEnv(key string) (string, string, error) {
        if v, ok := os.LookupEnv(key); ok {
                name, value, _ := strings.Cut(v, "=")
                return name, value, nil
        }
        return "", "", os.ErrNotExist
}

