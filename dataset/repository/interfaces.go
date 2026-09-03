package repository

import (
	"context"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

// DataSetRepository abstracts persistence of DataSet metadata.
type DataSetRepository interface {
	Save(ctx context.Context, ds *domain.DataSet) error
	FindByID(ctx context.Context, id string) (*domain.DataSet, error)
	FindByReferenceName(ctx context.Context, refName string) (*domain.DataSet, error)
	List(ctx context.Context, status string) ([]*domain.DataSet, error)
	Delete(ctx context.Context, id string) error
}
