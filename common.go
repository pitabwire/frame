package frame

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/xid"
	"gorm.io/gorm"
)

type BaseModelI interface {
	GetID() string
	GetVersion() uint
}

// BaseModel base table struct to be extended by other models.
type BaseModel struct {
	ID          string `gorm:"type:varchar(50);primary_key"`
	CreatedAt   time.Time
	ModifiedAt  time.Time
	Version     uint           `gorm:"DEFAULT 0"`
	TenantID    string         `gorm:"type:varchar(50);index:,composite:base_tenancy"`
	PartitionID string         `gorm:"type:varchar(50);index:,composite:base_tenancy"`
	AccessID    string         `gorm:"type:varchar(50);index:,composite:base_tenancy"`
	DeletedAt   gorm.DeletedAt `sql:"index"`
}

func (model *BaseModel) GetID() string {
	return model.ID
}

// GenID creates a new id for model if its not existent.
func (model *BaseModel) GenID(ctx context.Context) {
	if model.ID != "" {
		return
	}

	model.ID = GenerateID(ctx)

	authClaim := ClaimsFromContext(ctx)
	if authClaim == nil {
		return
	}

	if model.AccessID == "" && authClaim.GetAccessID() != "" {
		model.AccessID = authClaim.GetAccessID()
	}

	if model.PartitionID == "" && authClaim.GetPartitionID() != "" {
		model.PartitionID = authClaim.GetPartitionID()
	}

	if model.TenantID == "" && authClaim.GetTenantID() != "" {
		model.TenantID = authClaim.GetTenantID()
	}
}

// ValidXID Validates that the supplied string is an xid.
func (model *BaseModel) ValidXID(id string) bool {
	_, err := xid.FromString(id)
	return err == nil
}

func (model *BaseModel) GetVersion() uint {
	return model.Version
}

// BeforeSave Ensures we update a migrations time stamps.
func (model *BaseModel) BeforeSave(db *gorm.DB) error {
	return model.BeforeCreate(db)
}

func (model *BaseModel) BeforeCreate(db *gorm.DB) error {
	if model.Version <= 0 {
		model.CreatedAt = time.Now()
		model.ModifiedAt = time.Now()
		model.Version = 1
	}

	model.GenID(db.Statement.Context)
	return nil
}

// BeforeUpdate Updates time stamp every time we update status of a migration.
func (model *BaseModel) BeforeUpdate(_ *gorm.DB) error {
	model.ModifiedAt = time.Now()
	model.Version++
	return nil
}

func (model *BaseModel) CopyPartitionInfo(parent *BaseModel) {
	if parent == nil {
		return
	}

	if model.TenantID == "" || model.PartitionID == "" {
		model.TenantID = parent.TenantID
		model.PartitionID = parent.PartitionID
		model.AccessID = parent.AccessID
	}
}

// Migration Our simple table holding all the migration data.
type Migration struct {
	BaseModel

	Name        string `gorm:"type:text;uniqueIndex:idx_migrations_name"`
	Patch       string `gorm:"type:text"`
	RevertPatch string `gorm:"type:text"`
	AppliedAt   sql.NullTime
}

func GenerateID(_ context.Context) string {
	return xid.New().String()
}

// GetIP convenience method to extract the remote ip address from our inbound request.
func GetIP(r *http.Request) string {
	sourceIP := r.Header.Get("X-Forwarded-For")
	if sourceIP == "" {
		sourceIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	return sourceIP
}

// GetEnv Obtains the environment key or returns the default value.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// GetLocalIP convenince method that obtains the non localhost ip address for machine running app.
func GetLocalIP() string {
	addrs, _ := net.InterfaceAddrs()

	currentIP := ""

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			currentIP = ipnet.IP.String()
			break
		}
		if ipnet, ok := address.(*net.IPNet); ok {
			currentIP = ipnet.IP.String()
		}
	}

	return currentIP
}

// GetMacAddress convenience method to get some unique address based on the network interfaces the application is running on.
func GetMacAddress() string {
	currentIP := GetLocalIP()

	interfaces, _ := net.Interfaces()
	for _, interf := range interfaces {
		if addrs, err := interf.Addrs(); err == nil {
			for _, addr := range addrs {
				// only interested in the name with current IP address
				if strings.Contains(addr.String(), currentIP) {
					return fmt.Sprintf("%s:%s", interf.Name, interf.HardwareAddr.String())
				}
			}
		}
	}
	return ""
}
