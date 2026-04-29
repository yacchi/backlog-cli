//go:build (dragonfly && cgo) || (freebsd && cgo) || linux || netbsd || openbsd

package config

import (
	"errors"
	"fmt"

	dbus "github.com/godbus/dbus/v5"
)

var errCredentialSecretUnavailable = errors.New("credential secret store unavailable")

const (
	secretServiceName          = "org.freedesktop.secrets"
	secretServicePath          = "/org/freedesktop/secrets"
	secretServiceInterface     = "org.freedesktop.Secret.Service"
	secretCollectionsInterface = "org.freedesktop.Secret.Service.Collections"
	secretCollectionInterface  = "org.freedesktop.Secret.Collection"
	secretItemInterface        = "org.freedesktop.Secret.Item"
	secretSessionInterface     = "org.freedesktop.Secret.Session"
	secretPromptInterface      = "org.freedesktop.Secret.Prompt"

	secretLoginCollectionPath = "/org/freedesktop/secrets/collection/login"
	secretDefaultAliasPath    = "/org/freedesktop/secrets/aliases/default"
)

type systemCredentialSecretStore struct {
	service string
}

type secretServiceSession struct {
	conn *dbus.Conn
}

type secretServiceSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string `dbus:"content_type"`
}

func newSystemCredentialSecretStore() credentialSecretStore {
	return &systemCredentialSecretStore{service: credentialKeyringService}
}

func (s *systemCredentialSecretStore) IsAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}

	var hasOwner bool
	err = conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		Call("org.freedesktop.DBus.NameHasOwner", 0, secretServiceName).
		Store(&hasOwner)
	return err == nil && hasOwner
}

func (s *systemCredentialSecretStore) Get(ref string) (_ string, err error) {
	session, sessionPath, err := openSecretServiceSession()
	if err != nil {
		return "", err
	}
	defer func() {
		err = joinSecretServiceCloseError(err, session.close(sessionPath))
	}()

	item, err := session.findItem(s.stableAttributes(ref))
	if err != nil {
		return "", err
	}
	if err := session.unlock(item); err != nil {
		return "", err
	}

	secret, err := session.getSecret(item, sessionPath)
	if err != nil {
		return "", err
	}
	return string(secret.Value), nil
}

func (s *systemCredentialSecretStore) Set(ref string, secret string, meta credentialSecretMetadata) (err error) {
	session, sessionPath, err := openSecretServiceSession()
	if err != nil {
		return err
	}
	defer func() {
		err = joinSecretServiceCloseError(err, session.close(sessionPath))
	}()

	if err := session.deleteItemsByAttrs(s.stableAttributes(ref)); err != nil {
		if !errors.Is(err, errCredentialSecretNotFound) {
			return err
		}
	}

	attrs := s.stableAttributes(ref)
	attrs["profile"] = normalizeCredentialProfileName(meta.ProfileName)
	if meta.AuthType != "" {
		attrs["auth_type"] = string(meta.AuthType)
	}
	if meta.UserName != "" {
		attrs["user_name"] = meta.UserName
	}
	if meta.UserEmail != "" {
		attrs["user_email"] = meta.UserEmail
	}
	if meta.Space != "" {
		attrs["space"] = meta.Space
	}
	if meta.Domain != "" {
		attrs["domain"] = meta.Domain
	}
	if spaceURL := credentialSecretSpaceURL(meta); spaceURL != "" {
		attrs["space_url"] = spaceURL
	}

	return session.createItem(
		credentialSecretLabel(meta),
		attrs,
		secretServiceSecret{
			Session:     sessionPath,
			Parameters:  []byte{},
			Value:       []byte(secret),
			ContentType: "text/plain; charset=utf8",
		},
	)
}

func (s *systemCredentialSecretStore) Delete(ref string) (err error) {
	session, sessionPath, err := openSecretServiceSession()
	if err != nil {
		return err
	}
	defer func() {
		err = joinSecretServiceCloseError(err, session.close(sessionPath))
	}()

	return session.deleteItemsByAttrs(s.stableAttributes(ref))
}

func (s *systemCredentialSecretStore) stableAttributes(ref string) map[string]string {
	return map[string]string{
		"service": s.service,
		"ref":     ref,
	}
}

func openSecretServiceSession() (*secretServiceSession, dbus.ObjectPath, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, "", fmt.Errorf("%w: connect session bus: %w", errCredentialSecretUnavailable, err)
	}

	session := &secretServiceSession{conn: conn}
	sessionPath, err := session.open()
	if err != nil {
		return nil, "", err
	}

	if err := session.unlock(session.collectionPath()); err != nil {
		return nil, "", joinSecretServiceCloseError(err, session.close(sessionPath))
	}
	return session, sessionPath, nil
}

func (s *secretServiceSession) open() (dbus.ObjectPath, error) {
	var ignored dbus.Variant
	var sessionPath dbus.ObjectPath
	err := s.serviceObject().
		Call(secretServiceInterface+".OpenSession", 0, "plain", dbus.MakeVariant("")).
		Store(&ignored, &sessionPath)
	if err != nil {
		return "", fmt.Errorf("%w: open secret service session: %w", errCredentialSecretUnavailable, err)
	}
	return sessionPath, nil
}

func (s *secretServiceSession) close(sessionPath dbus.ObjectPath) error {
	if sessionPath == "" {
		return nil
	}
	if err := s.conn.Object(secretServiceName, sessionPath).Call(secretSessionInterface+".Close", 0).Err; err != nil {
		return fmt.Errorf("%w: close secret service session: %w", errCredentialSecretUnavailable, err)
	}
	return nil
}

func joinSecretServiceCloseError(err, closeErr error) error {
	if closeErr == nil {
		return err
	}
	if err == nil {
		return closeErr
	}
	return errors.Join(err, closeErr)
}

func (s *secretServiceSession) collectionPath() dbus.ObjectPath {
	obj := s.serviceObject()
	val, err := obj.GetProperty(secretCollectionsInterface)
	if err != nil {
		return dbus.ObjectPath(secretDefaultAliasPath)
	}
	for _, path := range val.Value().([]dbus.ObjectPath) {
		if path == dbus.ObjectPath(secretLoginCollectionPath) {
			return path
		}
	}
	return dbus.ObjectPath(secretDefaultAliasPath)
}

func (s *secretServiceSession) findItem(attrs map[string]string) (dbus.ObjectPath, error) {
	items, err := s.searchItems(attrs)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", errCredentialSecretNotFound
	}
	return items[0], nil
}

func (s *secretServiceSession) searchItems(attrs map[string]string) ([]dbus.ObjectPath, error) {
	var items []dbus.ObjectPath
	err := s.collectionObject().Call(secretCollectionInterface+".SearchItems", 0, attrs).Store(&items)
	if err != nil {
		return nil, fmt.Errorf("%w: search secret service items: %w", errCredentialSecretUnavailable, err)
	}
	return items, nil
}

func (s *secretServiceSession) deleteItemsByAttrs(attrs map[string]string) error {
	items, err := s.searchItems(attrs)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return errCredentialSecretNotFound
	}
	for _, item := range items {
		if err := s.deleteItem(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *secretServiceSession) createItem(label string, attrs map[string]string, secret secretServiceSecret) error {
	properties := map[string]dbus.Variant{
		secretItemInterface + ".Label":      dbus.MakeVariant(label),
		secretItemInterface + ".Attributes": dbus.MakeVariant(attrs),
	}

	var item dbus.ObjectPath
	var prompt dbus.ObjectPath
	err := s.collectionObject().
		Call(secretCollectionInterface+".CreateItem", 0, properties, secret, true).
		Store(&item, &prompt)
	if err != nil {
		return fmt.Errorf("%w: create secret service item: %w", errCredentialSecretUnavailable, err)
	}

	_, _, err = s.handlePrompt(prompt)
	if err != nil {
		return err
	}
	return nil
}

func (s *secretServiceSession) getSecret(itemPath, sessionPath dbus.ObjectPath) (*secretServiceSecret, error) {
	var secret secretServiceSecret
	err := s.conn.Object(secretServiceName, itemPath).
		Call(secretItemInterface+".GetSecret", 0, sessionPath).
		Store(&secret)
	if err != nil {
		return nil, fmt.Errorf("%w: get secret service payload: %w", errCredentialSecretUnavailable, err)
	}
	return &secret, nil
}

func (s *secretServiceSession) deleteItem(itemPath dbus.ObjectPath) error {
	var prompt dbus.ObjectPath
	err := s.conn.Object(secretServiceName, itemPath).
		Call(secretItemInterface+".Delete", 0).
		Store(&prompt)
	if err != nil {
		return fmt.Errorf("%w: delete secret service item: %w", errCredentialSecretUnavailable, err)
	}
	_, _, err = s.handlePrompt(prompt)
	return err
}

func (s *secretServiceSession) unlock(path dbus.ObjectPath) error {
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	err := s.serviceObject().
		Call(secretServiceInterface+".Unlock", 0, []dbus.ObjectPath{path}).
		Store(&unlocked, &prompt)
	if err != nil {
		return fmt.Errorf("%w: unlock secret service item: %w", errCredentialSecretUnavailable, err)
	}

	_, result, err := s.handlePrompt(prompt)
	if err != nil {
		return err
	}
	if paths, ok := result.Value().([]dbus.ObjectPath); ok {
		unlocked = append(unlocked, paths...)
	}
	if len(unlocked) == 0 {
		return fmt.Errorf("%w: unlock secret service item: no unlocked entries returned", errCredentialSecretUnavailable)
	}
	return nil
}

func (s *secretServiceSession) handlePrompt(prompt dbus.ObjectPath) (bool, dbus.Variant, error) {
	if prompt == dbus.ObjectPath("/") {
		return false, dbus.MakeVariant(""), nil
	}

	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(prompt),
		dbus.WithMatchInterface(secretPromptInterface),
	); err != nil {
		return false, dbus.MakeVariant(""), fmt.Errorf("%w: watch secret service prompt: %w", errCredentialSecretUnavailable, err)
	}
	defer func() {
		_ = s.conn.RemoveMatchSignal(
			dbus.WithMatchObjectPath(prompt),
			dbus.WithMatchInterface(secretPromptInterface),
		)
	}()

	signals := make(chan *dbus.Signal, 1)
	s.conn.Signal(signals)
	defer s.conn.RemoveSignal(signals)

	if err := s.conn.Object(secretServiceName, prompt).Call(secretPromptInterface+".Prompt", 0, "").Err; err != nil {
		return false, dbus.MakeVariant(""), fmt.Errorf("%w: prompt secret service unlock: %w", errCredentialSecretUnavailable, err)
	}

	signal := <-signals
	if signal == nil || signal.Name != secretPromptInterface+".Completed" {
		return false, dbus.MakeVariant(""), fmt.Errorf("%w: unexpected secret service prompt response", errCredentialSecretUnavailable)
	}

	return signal.Body[0].(bool), signal.Body[1].(dbus.Variant), nil
}

func (s *secretServiceSession) serviceObject() dbus.BusObject {
	return s.conn.Object(secretServiceName, secretServicePath)
}

func (s *secretServiceSession) collectionObject() dbus.BusObject {
	return s.conn.Object(secretServiceName, s.collectionPath())
}
