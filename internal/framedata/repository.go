package framedata

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
	readDB          *gorm.DB
	writeDB         *gorm.DB
	instanceCreator func() BaseModelI
}

func NewBaseRepository(readDB *gorm.DB, writeDB *gorm.DB, instanceCreator func() BaseModelI) *BaseRepository {
	return &BaseRepository{
		readDB:          readDB,
		writeDB:         writeDB,
		instanceCreator: instanceCreator,
	}
}

func (repo *BaseRepository) getReadDB() *gorm.DB {
	return repo.readDB
}

func (repo *BaseRepository) getWriteDB() *gorm.DB {
	return repo.writeDB
}

func (repo *BaseRepository) Delete(id string) error {
	deleteInstance := repo.instanceCreator()
	err := repo.GetByID(id, deleteInstance)
	if err != nil {
		return err
	}

	return repo.getWriteDB().Delete(deleteInstance).Error
}

func (repo *BaseRepository) GetByID(id string, result BaseModelI) error {
	return repo.getReadDB().Preload(clause.Associations).First(result, "id = ?", id).Error
}

func (repo *BaseRepository) GetLastestBy(properties map[string]any, result BaseModelI) error {
	db := repo.getReadDB()

	for key, value := range properties {
		db.Where(fmt.Sprintf("%s = ?", key), value)
	}

	return db.Last(result).Error
}

func (repo *BaseRepository) GetAllBy(properties map[string]any, result []BaseModelI) error {
	db := repo.getReadDB()

	for key, value := range properties {
		db.Where(fmt.Sprintf("%s = ?", key), value)
	}

	return db.Find(result).Error
}

func (repo *BaseRepository) Search(query string, searchFields []string, result []BaseModelI) error {
	db := repo.getReadDB()

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
		err := repo.getWriteDB().Create(instance).Error
		if err != nil {
			return err
		}
	} else {
		return repo.getWriteDB().Save(instance).Error
	}
	return nil
}
