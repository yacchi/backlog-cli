package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/yacchi/jubako/format/yaml"
	externalstore "github.com/yacchi/jubako/helper/coordinator/external-store"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/source/fs"
)

const (
	credentialMetadataLayerName = layer.Name("credentials_metadata")
	credentialRefPath           = "/secret_ref"
)

func newCredentialLayer(credentialsPath string) (layer.Layer, error) {
	metadata := layer.New(
		credentialMetadataLayerName,
		fs.New(credentialsPath, fs.WithFileMode(0600)),
		yaml.New(),
	)
	secretStore := newCredentialKeyringStore(newCredentialSecretStore())
	keyringAvailable := secretStore.IsAvailable()

	return externalstore.NewMap[*Credential](LayerCredentials, externalstore.MapConfig[*Credential]{
		RootPath:         PathCredential,
		Metadata:         optionalLoadLayer{Layer: metadata},
		External:         secretStore,
		RefPath:          credentialRefPath,
		ExternalTagKey:   "storage",
		ExternalTagValue: "keyring",
		RouteForEntry: func(ctx externalstore.RouteContext[*Credential]) (externalstore.Route, error) {
			backend, err := resolveCredentialBackend(ctx.Logical, keyringAvailable)
			if err != nil {
				return externalstore.Route{}, err
			}

			ref := ctx.ExistingRef
			if ref == "" {
				ref = credentialRef(ctx.Key)
			}

			return externalstore.Route{
				UseExternal: backend == CredentialBackendKeyring,
				Ref:         ref,
			}, nil
		},
	})
}

func resolveCredentialBackend(logical map[string]any, keyringAvailable bool) (CredentialBackend, error) {
	raw, _ := jsonptr.GetPath(logical, PathAuthCredentialBackend)
	value, _ := raw.(string)
	backend, err := NormalizeCredentialBackend(value)
	if err != nil {
		return "", fmt.Errorf("invalid auth.credential_backend: %w", err)
	}
	if backend == CredentialBackendAuto {
		if keyringAvailable {
			return CredentialBackendKeyring, nil
		}
		return CredentialBackendFile, nil
	}
	return backend, nil
}

func credentialRef(profileName string) string {
	return "profile:" + normalizeCredentialProfileName(profileName)
}

type credentialKeyringStore struct {
	backend credentialSecretStore
}

type optionalLoadLayer struct {
	layer.Layer
}

func (l optionalLoadLayer) Load(ctx context.Context) (map[string]any, error) {
	data, err := l.Layer.Load(ctx)
	if errors.Is(err, source.ErrNotExist) {
		return map[string]any{}, nil
	}
	return data, err
}

func newCredentialKeyringStore(backend credentialSecretStore) *credentialKeyringStore {
	return &credentialKeyringStore{backend: backend}
}

func (s *credentialKeyringStore) IsAvailable() bool {
	return s.backend != nil && s.backend.IsAvailable()
}

func (s *credentialKeyringStore) Get(_ context.Context, c externalstore.ExternalContext[*Credential]) (map[string]any, error) {
	secret, err := s.backend.Get(c.ExternalKey)
	if err != nil {
		if errors.Is(err, errCredentialSecretNotFound) {
			return nil, externalstore.NewNotExistError(c.ExternalKey, err)
		}
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(secret), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *credentialKeyringStore) Set(_ context.Context, c externalstore.ExternalContext[*Credential], value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.backend.Set(c.ExternalKey, string(data), newCredentialSecretMetadata(c.Key, c.After))
}

func (s *credentialKeyringStore) Delete(_ context.Context, c externalstore.ExternalContext[*Credential]) error {
	err := s.backend.Delete(c.ExternalKey)
	if errors.Is(err, errCredentialSecretNotFound) {
		return nil
	}
	return err
}
