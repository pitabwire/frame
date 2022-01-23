package frame

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BaseRepository struct {
	readDb          *gorm.DB
	writeDb         *gorm.DB
	instanceCreator func() BaseModelI
}

func (repo *BaseRepository) getReadDb() *gorm.DB {
	return repo.readDb
}

func (repo *BaseRepository) getWriteDb() *gorm.DB {
	return repo.writeDb
}

func (repo *BaseRepository) Delete(id string) error {
	deleteInstance, err := repo.GetByID(id)
	if err != nil {
		return err
	}

	return repo.getWriteDb().Delete(deleteInstance).Error

}

func (repo *BaseRepository) GetByID(id string) (BaseModelI, error) {
	getInstance := repo.instanceCreator()
	err := repo.getReadDb().Preload(clause.Associations).First(getInstance, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return getInstance, nil
}

func (repo *BaseRepository) GetLastestBy(properties map[string]interface{}) (BaseModelI, error) {
	getInstance := repo.instanceCreator()

	db := repo.getReadDb()

	for key, value := range properties {
		db.Where(fmt.Sprintf("%s = ?", key), value)
	}

	err := db.Last(getInstance).Error
	if err != nil {
		return nil, err
	}
	return getInstance, nil
}

func (repo *BaseRepository) GetAllBy(properties map[string]interface{}, instanceList interface{}) error {

	db := repo.getReadDb()

	for key, value := range properties {
		db.Where(fmt.Sprintf("%s = ?", key), value)
	}

	return db.Find(instanceList).Error
}

func (repo *BaseRepository) Search(query string, searchFields []string, instanceList interface{}) error {

	db := repo.getReadDb()

	for i, field := range searchFields {
		if i == 0 {
			db.Where(fmt.Sprintf("%s iLike ?", field), query)
		} else {
			db.Or(fmt.Sprintf(" %s iLike ?", field), query)
		}
	}

	return db.Find(instanceList).Error
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
