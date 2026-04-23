package logx

import (
	"fmt"
	"log"
	"strings"
)

// Info emite logs estructurados simples en formato key=value.
func Info(message string, fields map[string]interface{}) {
	log.Printf("level=INFO msg=%q %s", message, formatFields(fields))
}

// Error emite logs estructurados simples en formato key=value.
func Error(message string, err error, fields map[string]interface{}) {
	merged := map[string]interface{}{}
	for k, v := range fields {
		merged[k] = v
	}
	if err != nil {
		merged["error"] = err.Error()
	}
	log.Printf("level=ERROR msg=%q %s", message, formatFields(merged))
}

func formatFields(fields map[string]interface{}) string {
	if len(fields) == 0 {
		return ""
	}

	parts := make([]string, 0, len(fields))
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%q", sanitize(k), fmt.Sprint(v)))
	}
	return strings.Join(parts, " ")
}

func sanitize(key string) string {
	return strings.ReplaceAll(key, " ", "_")
}
