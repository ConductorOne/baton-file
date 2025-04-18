package local

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	v1 "github.com/conductorone/baton-sdk/pb/c1/connectorapi/baton/v1"
	"github.com/conductorone/baton-sdk/pkg/provisioner"
	"github.com/conductorone/baton-sdk/pkg/tasks"
	"github.com/conductorone/baton-sdk/pkg/types"
)

type localCredentialRotator struct {
	dbPath string
	o      sync.Once

	resourceId   string
	resourceType string
}

func (m *localCredentialRotator) GetTempDir() string {
	return ""
}

func (m *localCredentialRotator) ShouldDebug() bool {
	return false
}

func (m *localCredentialRotator) Next(ctx context.Context) (*v1.Task, time.Duration, error) {
	var task *v1.Task
	m.o.Do(func() {
		task = &v1.Task{
			TaskType: &v1.Task_RotateCredentials{},
		}
	})
	return task, 0, nil
}

func (m *localCredentialRotator) Process(ctx context.Context, task *v1.Task, cc types.ConnectorClient) error {
	ctx, span := tracer.Start(ctx, "localCredentialRotator.Process", trace.WithNewRoot())
	defer span.End()

	accountManager := provisioner.NewCredentialRotator(cc, m.dbPath, m.resourceId, m.resourceType)

	err := accountManager.Run(ctx)
	if err != nil {
		return err
	}

	err = accountManager.Close(ctx)
	if err != nil {
		return err
	}

	return nil
}

// NewGranter returns a task manager that queues a sync task.
func NewCredentialRotator(ctx context.Context, dbPath string, resourceId string, resourceType string) tasks.Manager {
	return &localCredentialRotator{
		dbPath:       dbPath,
		resourceId:   resourceId,
		resourceType: resourceType,
	}
}
