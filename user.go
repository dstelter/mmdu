package main

import (
	"database/sql"
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"reflect"
	"regexp"
	"strings"
)

type User struct {
	Username    string
	Host        string
	Password    string
	Database    string
	Table       string
	Privileges  []string
	GrantOption bool
}

type UsersConfig struct {
	User []User
}

func (u *User) dropUser(db *sql.DB) bool {
	_, err := db.Exec("DROP USER " + u.Username + "@" + u.Password)
	if err != nil {
		return false
	}
	return true
}

func (u *User) addUser(db *sql.DB) bool {
	_, err := db.Exec("GRANT " + strings.Join(u.Privileges, ", ") + " ON " + u.Database + "." + u.Table + " TO '" +
		u.Username + "'@'" + u.Host + "' IDENTIFIED BY PASSWORD '" + u.Password + "'")
	if err != nil {
		return false
	}
	return true
}

func getUserFromDatabase(username, host string, db *sql.DB) (User, error) {

	var grantPriv, grantLine string
	var user User

	err := db.QueryRow(selectUsersCurrentData, username, host).Scan(&user.Username, &user.Host, &user.Password, &grantPriv)
	if err != nil {
		fmt.Println("Error querying "+selectUsersCurrentData+": ", err.Error())
		return user, err
	} else {
		if grantPriv == "Y" {
			user.GrantOption = true
		}
		err = db.QueryRow(fmt.Sprint("SHOW GRANTS FOR " + username + "@" + "'" + host + "'")).Scan(&grantLine)
		if err != nil {
			fmt.Println(err.Error())
			return user, err
		} else {
			re := regexp.MustCompile("GRANT (.*) ON (.*)\\.(.*) TO.*")
			user.Privileges = strings.Split(re.ReplaceAllString(grantLine, "$1"), ",")
			user.Database = re.ReplaceAllString(grantLine, "$2")
			user.Table = re.ReplaceAllString(grantLine, "$3")
		}
	}
	return user, nil
}

func getAllUsersFromDB(db *sql.DB) []User {
	var users []User
	rows, err := db.Query(selectAllUsers)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var username, host string
		if err := rows.Scan(&username, &host); err != nil {
			log.Fatal(err)
		}
		user, err := getUserFromDatabase(username, host, db)
		if err == nil {
			users = append(users, user)
		}

	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	return users
}

func getAllUsersFromConfig() []User {
	var users UsersConfig
	if _, err := toml.DecodeFile("./mmdu.toml", &users); err != nil {
		fmt.Println("Failed to parse config file", err.Error())
	}
	return users.User
}

func getUsersToRemove(usersFromConf, usersFromDB []User) []User {
	var usersToRemove []User
	for _, userDB := range usersFromDB {
		var found bool
		for _, userConf := range usersFromConf {
			if reflect.DeepEqual(userConf, userDB) {
				found = true
				break
			}
		}
		if !found {
			usersToRemove = append(usersToRemove, userDB)
		}
	}
	return usersToRemove
}

func getUsersToAdd(usersFromConf, usersFromDB []User) []User {
	var usersToAdd []User
	for _, userConf := range usersFromConf {
		var found bool
		for _, userDB := range usersFromDB {
			if reflect.DeepEqual(userConf, userDB) {
				found = true
				break
			}
		}
		if !found {
			usersToAdd = append(usersToAdd, userConf)
		}
	}
	return usersToAdd
}
