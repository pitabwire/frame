package frame

import (
	"github.com/rs/xid"
	"gorm.io/gorm"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Migration Our simple table holding all the migration data
type BaseModel struct {
	ID         string `gorm:"type:varchar(50);primary_key"`
	CreatedAt  time.Time
	ModifiedAt time.Time
	Version    uint32         `gorm:"DEFAULT 0"`
	DeletedAt  gorm.DeletedAt `sql:"index"`
}

// BeforeCreate Ensures we update a migrations time stamps
func (model *BaseModel) BeforeCreate(db *gorm.DB) error {
	model.ID = xid.New().String()
	model.CreatedAt = time.Now()
	model.ModifiedAt = time.Now()
	model.Version = 1
	return nil
}

// BeforeUpdate Updates time stamp every time we update status of a migration
func (model *BaseModel) BeforeUpdate(db *gorm.DB) error {
	model.ModifiedAt = time.Now()
	model.Version = 1
	return nil
}

// Migration Our simple table holding all the migration data
type Migration struct {
	BaseModel

	Name      string `gorm:"type:varchar(50);unique_index"`
	Patch     string `gorm:"type:text"`
	AppliedAt *time.Time
}



func GetIp(r *http.Request) string {
	sourceIp := r.Header.Get("X-FORWARDED-FOR")
	if sourceIp == "" {
		sourceIp, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	return sourceIp
}

// GetEnv Obtains the environment key or returns the default value
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetLocalIP() string {

	addrs, _ := net.InterfaceAddrs()

	currentIP := ""

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			currentIP = ipnet.IP.String()
			break
		} else {
			currentIP = ipnet.IP.String()
		}
	}

	return currentIP

}

func GetMacAddress() string {

	currentIP := GetLocalIP()

	interfaces, _ := net.Interfaces()
	for _, interf := range interfaces {

		if addrs, err := interf.Addrs(); err == nil {
			for _, addr := range addrs {

				// only interested in the name with current IP address
				if strings.Contains(addr.String(), currentIP) {
					return interf.HardwareAddr.String()
				}
			}
		}
	}

	return ""

}
