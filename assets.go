package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	base := make([]byte, 32)
	_, err := rand.Read(base)
	if err != nil {
		panic("failed to generate random bytes")
	}
	id := base64.RawURLEncoding.EncodeToString(base)

	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", id, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func reduceAspectRatio(width, height int) string {
	if width == 0 || height == 0 {
		return "unknown"
	}
	g := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/g, height/g)
}

func classifyAspectRatio(width, height int) string {
	ratio := reduceAspectRatio(width, height)

	switch ratio {
	case "16:9", "4:3", "3:2", "21:9":
		return "landscape"
	case "9:16", "3:4", "2:3":
		return "portrait"
	case "1:1":
		return "square"
	default:
		// fallback: infer based on dimension
		if width > height {
			return "landscape"
		} else if height > width {
			return "portrait"
		}
		return "other"
	}
}

type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func (cfg apiConfig) getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run ffprobe: %w", err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return "", fmt.Errorf("failed to unmarshal ffprobe output: %w", err)
	}

	if len(parsed.Streams) == 0 {
		return "", fmt.Errorf("invalid or missing stream data")
	}

	width := parsed.Streams[0].Width
	height := parsed.Streams[0].Height

	return classifyAspectRatio(width, height), nil
}
