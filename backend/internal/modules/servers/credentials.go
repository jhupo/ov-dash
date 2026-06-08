package servers

import "context"

type CredentialStore interface {
	Put(ctx context.Context, scope string, name string, plaintext string) (string, error)
	Get(ctx context.Context, id string) (string, error)
	Delete(ctx context.Context, id string) error
	DeleteNamed(ctx context.Context, scope string, name string) error
}

type CredentialResolver struct {
	store CredentialStore
}

type preparedCredentials struct {
	Password           *string
	PrivateKey         *string
	PasswordSecretID   string
	PrivateKeySecretID string
}

func NewCredentialResolver(store CredentialStore) *CredentialResolver {
	return &CredentialResolver{store: store}
}

func (r *CredentialResolver) PrepareSave(ctx context.Context, input SaveInput) (preparedCredentials, error) {
	prepared := preparedCredentials{
		Password:   input.Password,
		PrivateKey: input.PrivateKey,
	}
	if r == nil || r.store == nil {
		return prepared, nil
	}

	scope := serverCredentialScope(input.ID)
	if input.ClearSecret {
		_ = r.store.DeleteNamed(ctx, scope, "password")
		_ = r.store.DeleteNamed(ctx, scope, "private_key")
	}

	if input.Password != nil {
		if *input.Password == "" {
			_ = r.store.DeleteNamed(ctx, scope, "password")
		} else {
			id, err := r.store.Put(ctx, scope, "password", *input.Password)
			if err != nil {
				return preparedCredentials{}, err
			}
			prepared.PasswordSecretID = id
			prepared.Password = emptyStringPointer()
		}
	}
	if input.PrivateKey != nil {
		if *input.PrivateKey == "" {
			_ = r.store.DeleteNamed(ctx, scope, "private_key")
		} else {
			id, err := r.store.Put(ctx, scope, "private_key", *input.PrivateKey)
			if err != nil {
				return preparedCredentials{}, err
			}
			prepared.PrivateKeySecretID = id
			prepared.PrivateKey = emptyStringPointer()
		}
	}
	return prepared, nil
}

func (r *CredentialResolver) Resolve(ctx context.Context, item Connection) Connection {
	if r == nil || r.store == nil {
		return item
	}
	if item.PasswordSecretID != "" {
		if value, err := r.store.Get(ctx, item.PasswordSecretID); err == nil {
			item.Password = value
		}
	}
	if item.PrivateKeySecretID != "" {
		if value, err := r.store.Get(ctx, item.PrivateKeySecretID); err == nil {
			item.PrivateKey = value
		}
	}
	return item
}

func (r *CredentialResolver) Delete(ctx context.Context, item Connection) error {
	if r == nil || r.store == nil {
		return nil
	}
	if item.PasswordSecretID != "" {
		_ = r.store.Delete(ctx, item.PasswordSecretID)
	}
	if item.PrivateKeySecretID != "" {
		_ = r.store.Delete(ctx, item.PrivateKeySecretID)
	}
	return nil
}

func serverCredentialScope(id string) string {
	return "server_connections:" + id
}

func emptyStringPointer() *string {
	empty := ""
	return &empty
}
