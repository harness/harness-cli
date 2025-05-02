package migratable

import "context"

type Job interface {
	Info() string
	Pre(ctx context.Context) error
	Migrate(ctx context.Context) error
	Post(ctx context.Context) error
}
