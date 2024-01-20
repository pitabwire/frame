package frame

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BaseRepositoryI interface {
	GetByID(id string, result BaseModelI) error
	Delete(id string) error
	Save(instance BaseModelI) error
}

type BaseRepository struct {
	readDb          *gorm.DB
	writeDb         *gorm.DB
	instanceCreator func() BaseModelI
}

func NewBaseRepository(readDb *gorm.DB, writeDB *gorm.DB, instanceCreator func() BaseModelI) *BaseRepository {
	return &BaseRepository{
		readDb:          readDb,
		writeDb:         writeDB,
		instanceCreator: instanceCreator,
	}
}

func (repo *BaseRepository) getReadDb() *gorm.DB {
	return repo.readDb
}

func (repo *BaseRepository) getWriteDb() *gorm.DB {
	return repo.writeDb
}

func (repo *BaseRepository) Delete(id string) error {
	deleteInstance := repo.instanceCreator()
	err := repo.GetByID(id, deleteInstance)
	if err != nil {
		return err
	}

	return repo.getWriteDb().Delete(deleteInstance).Error

}

func (repo *BaseRepository) GetByID(id string, result BaseModelI) error {
	return repo.getReadDb().Preload(clause.Associations).First(result, "id = ?", id).Error
}

func (repo *BaseRepository) GetLastestBy(properties map[string]any, result BaseModelI) error {

	db := repo.getReadDb()

	for key, value := range properties {
		db.Where(fmt.Sprintf("%s = ?", key), value)
	}

	return db.Last(result).Error
}

func (repo *BaseRepository) GetAllBy(properties map[string]any, result []BaseModelI) error {

	db := repo.getReadDb()

	for key, value := range properties {
		db.Where(fmt.Sprintf("%s = ?", key), value)
	}

	return db.Find(result).Error
}

func (repo *BaseRepository) Search(query string, searchFields []string, result []BaseModelI) error {

	db := repo.getReadDb()

	for i, field := range searchFields {
		if i == 0 {
			db.Where(fmt.Sprintf("%s iLike ?", field), query)
		} else {
			db.Or(fmt.Sprintf(" %s iLike ?", field), query)
		}
	}

	return db.Find(result).Error
}

func (repo *BaseRepository) Save(instance BaseModelI) error {

	if instance.GetVersion() <= 0 {

		err := repo.getWriteDb().Create(instance).Error
		if err != nil {
			return err
		}
	} else {
		return repo.getWriteDb().Save(instance).Error
	}
	return nil
}
