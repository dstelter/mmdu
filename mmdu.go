package main

import (
	"flag"
	"fmt"
	"github.com/BurntSushi/toml"
	_ "github.com/go-sql-driver/mysql"
	"os"
)

type Config struct {
	Access   Access
	Database []Database
	User     []User
}

const selectAllUsers = "SELECT User, Host FROM mysql.user"
const showAllDatabases = "SHOW DATABASES"

func main() {
	var execute bool
	var configFile string

	flag.BoolVar(&execute, "e", false, "Execute. If specified - changes will be applied")
	flag.StringVar(&configFile, "c", "/etc/mmdu/mmdu.toml", "Path to config file.")
	flag.Parse()

	var conf Config
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		fmt.Println("Failed to parse config file", err.Error())
		os.Exit(1)
	}

	defaultDatabases := []Database{Database{"information_schema"}, Database{"mysql"}, Database{"performance_schema"}}
	validatedUsers, err := validateUsers(conf.User)
	if err != nil {
		fmt.Println("Error during validation of user list:", err.Error())
		os.Exit(1)
	}

	db := conf.Access.connectAndCheck()

	usersFromDB, err := getAllUsersFromDB(db)
	if err != nil {
		fmt.Println("Failed during execution " + selectAllUsers, err.Error())
		os.Exit(2)
	}

	databasesFromDB, err := getDatabasesFromDB(db)
	if err != nil {
		fmt.Println("Failed during execution " + showAllDatabases, err.Error())
		os.Exit(2)
	}

	// Merge from 3 sources: default, from DbConfig and from UserConfg
	databasesFromConf := removeDuplicateDatabases(
		append(append(defaultDatabases, conf.Database...), getDatabasesFromUsers(validatedUsers)...))

	tx, err := db.Begin()
	if err != nil {
		fmt.Println("Failed to start transaction", err.Error())
		os.Exit(2)
	}

	usersToRemove := getUsersToRemove(validatedUsers, usersFromDB)
	usersToAdd := getUsersToAdd(validatedUsers, usersFromDB)

	databasesToRemove := getDatabasesToRemove(databasesFromConf, databasesFromDB)
	databasesToAdd := getDatabasesToAdd(databasesFromConf, databasesFromDB)

	for _, user := range usersToRemove {
		if !user.dropUser(tx, execute) {
			tx.Rollback()
		}
	}

	for _, user := range usersToAdd {
		if !user.addUser(tx, execute) {
			tx.Rollback()
		}
	}

	for _, database := range databasesToRemove {
		if !database.dropDatabase(tx, execute) {
			tx.Rollback()
		}
	}

	for _, database := range databasesToAdd {
		if !database.addDatabase(tx, execute) {
			tx.Rollback()
		}
	}

	tx.Commit()
}
