package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/go-archive"
	"github.com/sirupsen/logrus"
)

// BuildImage builds a Docker image from a context directory.
func BuildImage(ctx context.Context, dockerClient *client.Client, contextDir, dockerfile, imageName string, logger *logrus.Logger) error {
	tar, err := archive.TarWithOptions(contextDir, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer tar.Close()

	options := types.ImageBuildOptions{
		Tags:       []string{imageName},
		Remove:     true,
		Dockerfile: dockerfile,
	}

	response, err := dockerClient.ImageBuild(ctx, tar, options)
	if err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(response.Body)
	for {
		var msg jsonmessage.JSONMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read build output: %w", err)
		}
		if msg.Error != nil {
			return fmt.Errorf("docker build error: %s", msg.Error.Message)
		}
		if msg.Stream != "" && logger != nil {
			logger.Infof("build: %s", msg.Stream)
		}
	}

	return nil
}
