/*
config
Redis
环境变量
LLM配置
Embedding配置


database.go:封装与 MySQL 数据库建立连接的逻辑，并将这个连接对象（*gorm.DB）返回给调用方

*/

package config
import(
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

//go和mysql之间的启动函数
//*gorm.DB:GORM的核心对象，靠它增删改查
func InitDB()(*gorm.DB,error){

	//dsn是一个URL格式：
	//路径参数：gorag:gorag123456@tcp(127.0.0.1:3306)/gorag：指明用户名和密码，ip和端口，以及具体的数据库名
	//查询参数：?charset=utf8mb4&parseTime=True&loc=Local
	dsn:="gorag:gorag123456@tcp(127.0.0.1:3306)/gorag?charset=utf8mb4&parseTime=True&loc=Local"

	//gorm.Open(mysql.Open(dsn), &gorm.Config{})：GORM官方提供的标准连接方法
	//mysql.Open(dsn)：mysql的解析器，将dsn转化成mysql对应的网络连接参数

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	fmt.Println("MySQL connected successfully")

	//！！！返回*gorm.DB，整个程序操作数据库的入口
	return db, nil
}