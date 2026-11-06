package session

import (
	"crypto/subtle"
	"encoding/gob"

	"x-ui/database"
	"x-ui/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUserKey = "LOGIN_USER"
	defaultPath  = "/"
)

func init() {
	gob.Register(model.User{})
}

func SetLoginUser(c *gin.Context, user *model.User) {
	if user == nil {
		return
	}
	s := sessions.Default(c)
	s.Set(loginUserKey, *user)
}

func SetMaxAge(c *gin.Context, maxAge int) {
	s := sessions.Default(c)
	s.Options(sessions.Options{
		Path:     defaultPath,
		MaxAge:   maxAge,
		HttpOnly: true,
	})
}

func GetLoginUser(c *gin.Context) *model.User {
	s := sessions.Default(c)
	obj := s.Get(loginUserKey)
	if obj == nil {
		return nil
	}
	user, ok := obj.(model.User)
	if !ok {

		s.Delete(loginUserKey)
		return nil
	}
	// Credential changes revoke existing cookies, while upgrades preserve valid sessions.
	db := database.GetDB()
	var current model.User
	if db == nil || db.First(&current, user.Id).Error != nil ||
		current.Username != user.Username ||
		subtle.ConstantTimeCompare([]byte(current.Password), []byte(user.Password)) != 1 {
		return nil
	}
	return &current
}

func IsLogin(c *gin.Context) bool {
	return GetLoginUser(c) != nil
}

func ClearSession(c *gin.Context) {
	s := sessions.Default(c)
	s.Clear()
	s.Options(sessions.Options{
		Path:     defaultPath,
		MaxAge:   -1,
		HttpOnly: true,
	})
}
