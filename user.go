package main

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"io"
	"reflect"
	"strings"
	"errors"
)

type User struct {
	Username       string
	Network        string
	Password       string
	HashedPassword string
	Permissions    []Permission
	GrantOption    bool
}

func (u *User) calcUserHashPassword() {
	h := sha1.New()
	io.WriteString(h, u.Password)
	h2 := sha1.New()
	h2.Write(h.Sum(nil))

	u.HashedPassword = strings.ToUpper(strings.Replace(fmt.Sprintf("*% x", h2.Sum(nil)), " ", "", -1))
}

func (u *User) compare(usr *User) bool {
	if u.Username == usr.Username && u.Network == usr.Network && u.HashedPassword == usr.HashedPassword &&
		len(u.Permissions) == len(usr.Permissions) {
		for _, up := range u.Permissions {
			var found bool
			for _, uperm := range usr.Permissions {
				if reflect.DeepEqual(up, uperm) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	} else {
		return false
	}

}

func (u *User) dropUser(tx *sql.Tx, execute bool) bool {
	query := "DROP USER '" + u.Username + "'@'" + u.Network + "'"
	if execute {
		_, err := tx.Exec(query)
		if err != nil {
			return false
		}
	} else {
		fmt.Println(query)
	}

	return true
}

func (u *User) addUser(tx *sql.Tx, execute bool) bool {

	for _, permission := range u.Permissions {
		database := permission.Database
		table := permission.Table
		privileges := permission.Privileges

		if strings.Contains(database, "%") {
			database = "`" + database + "`"
		}

		if strings.Contains(table, "%") {
			table = "`" + table + "`"
		}

		query := "GRANT " + strings.Join(privileges, ", ") + " ON " + database + "." + table + " TO '" +
		u.Username + "'@'" + u.Network + "' IDENTIFIED BY PASSWORD '" + u.HashedPassword + "'"

		if u.GrantOption {
			query += " WITH GRANT OPTION"
		}

		if execute {
			_, err := tx.Exec(query)
			if err != nil {
				return false
			}
		} else {
			fmt.Println(query)
		}
	}

	return true
}

func getUserFromDatabase(username, network string, db *sql.DB) (User, error) {

	var grantPriv string
	var user User
	query := "SELECT User, Host, Password, Grant_priv FROM mysql.user WHERE User='" + username + "' and Host='" + network + "'"
	err := db.QueryRow(query).Scan(&user.Username, &user.Network, &user.HashedPassword, &grantPriv)
	if err != nil {
		fmt.Println("Error querying " + query + ": ", err.Error())
		return user, err
	} else {
		if grantPriv == "Y" {
			user.GrantOption = true
		}
		rows, err := db.Query("SHOW GRANTS FOR '" + username + "'@'" + network + "'")
		if err != nil {
			fmt.Println(err.Error())
			return user, err
		} else {
			defer rows.Close()
			var grantLines []string
			for rows.Next() {
				var grantLine string
				if err := rows.Scan(&grantLine); err != nil {
					return user, err
				}
				grantLines = append(grantLines, grantLine)
			}
			var permissions []Permission
			if len(grantLines) == 1 {
				var tmpPerm Permission
				tmpPerm.parseUserFromGrantLine(grantLines[0])
				permissions = append(permissions, tmpPerm)

			} else {
				for _, grantLine := range grantLines[1:] {
					var tmpPerm Permission
					tmpPerm.parseUserFromGrantLine(grantLine)
					permissions = append(permissions, tmpPerm)
				}
			}
			user.Permissions = permissions
		}
	}
	return user, nil
}

func getAllUsersFromDB(db *sql.DB) ([]User, error) {
	var users []User
	rows, err := db.Query(selectAllUsers)
	if err != nil {
		return users, err
	}
	defer rows.Close()

	for rows.Next() {
		var username, host string
		if err := rows.Scan(&username, &host); err != nil {
			return users, err
		}
		user, err := getUserFromDatabase(username, host, db)
		if err != nil {
			return users, err
		} else {
			users = append(users, user)
		}
	}
	if err := rows.Err(); err != nil {
		return users, err
	}

	return users, nil
}

func validateUsers(users []User) ([]User, error) {
	var resultUsers []User
	for _, u := range users {
		if u.Username != "" && u.Network != "" && len(u.Permissions) > 0 && (u.Password != "" || u.HashedPassword != "") {
			for _, permission := range u.Permissions {
				if permission.Database == "" || permission.Database == "" || len(permission.Privileges) == 0 {
					return resultUsers, errors.New("Permissions for user set incorrectly")
				}
			}
			if u.Password != "" {
				u.calcUserHashPassword()
			}
			resultUsers = append(resultUsers, u)
		} else {
			errorDescription := "Username, Network, Permissions, Passowrd or HashedPassword must be set set"
			return resultUsers, errors.New(errorDescription)
		}
	}
	return resultUsers, nil
}

func getUsersToRemove(usersFromConf, usersFromDB []User) []User {
	var usersToRemove []User
	for _, userDB := range usersFromDB {
		var found bool
		for _, userConf := range usersFromConf {
			if userConf.compare(&userDB) {
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
			if userConf.compare(&userDB) {
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