package jfrog

import (
	"context"
	"fmt"
	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/reg/filter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"harness/module/ar/adapter"
	"harness/module/ar/types"
	"strings"
)

// factory section
type factory struct {
}

func init() {
	adapterType := types.JFROG
	if err := adapter.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adapter.Adapter, error) {
	return newAdapter(config)
}

func newAdapter(config types.RegistryConfig) (adapter.Adapter, error) {
	return newClient(&config), nil
}

// ListArtifacts lists all artifacts from a specified registry in JFrog
func (a *client) ListArtifacts(registry string, artifactType types.ArtifactType) ([]types.Artifact, error) {
	key, repoName := "", ""
	s := strings.Split(registry, "/")
	if len(s) > 1 {
		key = s[0]
		repoName = strings.Join(s[1:], "/")
	}
	url := fmt.Sprintf("%s/artifactory/api/docker/%s", a.client.url, key)
	regClient := registry.NewClientWithAuthorizer(url, basic.NewAuthorizer(a.client.username, a.client.password),
		a.client.insecure)
	tags, err := regClient.ListTags(repoName)
	if err != nil {
		return nil, err
	}

	var artifacts []*model.Artifact
	for _, tag := range tags {
		artifacts = append(artifacts, &model.Artifact{
			Tags: []string{tag},
		})
	}
	return filter.DoFilterArtifacts(artifacts, filters)

	artifacts, err := a.listArtifacts(repo.Name, filters)
	if err != nil {
		return fmt.Errorf("failed to list artifacts of repository %s: %v", repo.Name, err)
	}
	if len(artifacts) == 0 {
		return nil, nil
	}
}

// CreateRegistry creates a registry in JFrog
func (a *client) PrepareForPush(registryID string, packageType string) (string, error) {
	log.Errorf("[JFROG] not implemented create registry %s for package type %s", registryID, packageType)
}

// PullArtifact pulls an artifact from the JFrog registry
func (a *client) PullArtifact(registry string, artifact types.Artifact) ([]byte, error) {
	// Implementation for pulling an artifact from JFrog
	// This would make an API call to the JFrog endpoint

	// Placeholder implementation
	return []byte("jfrog artifact data"), nil
}

// PushArtifact pushes an artifact to the JFrog registry
func (a *client) PushArtifact(registry string, artifact types.Artifact, data []byte) error {
	// Implementation for pushing an artifact to JFrog
	// This would make an API call to the JFrog endpoint

	// Placeholder implementation
	return nil
}
