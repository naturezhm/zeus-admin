package middleware

import (
	"errors"
	"fmt"
	"github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"net/http"
	"time"
	"zeus/pkg/api/domain/account"
	"zeus/pkg/api/dto"
	"zeus/pkg/api/log"
	"zeus/pkg/api/model"
	"zeus/pkg/api/service"
)

var accountService = service.UserService{}

//todo : 用单独的claims model去掉user model
func JwtAuth(LoginType int) *jwt.GinJWTMiddleware {
	jwtMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            "Jwt",
		SigningAlgorithm: "RS256",
		PubKeyFile:       viper.GetString("jwt.key.public"),
		PrivKeyFile:      viper.GetString("jwt.key.private"),
		Timeout:          time.Hour * 24,
		MaxRefresh:       time.Hour * 24 * 90,
		IdentityKey:      "id",
		LoginResponse:    LoginResponse,
		PayloadFunc: func(data interface{}) jwt.MapClaims {
			if v, ok := data.(model.UserClaims); ok {
				return jwt.MapClaims{
					"id":    v.Id,
					"name":  v.Name,
					"uid":   v.Id,
					"uname": v.Name,
				}
			}
			return jwt.MapClaims{}
		},
		IdentityHandler: func(c *gin.Context) interface{} {
			claims := jwt.ExtractClaims(c)
			return model.UserClaims{
				Name: claims["name"].(string),
				Id:   int(claims["id"].(float64)),
			}
		},
		Authenticator: func(c *gin.Context) (interface{}, error) {
			if LoginType == account.LoginOAuth { //OAuth
				return AuthenticatorOAuth(c)
			}
			return Authenticator(c, LoginType)
		},
		Authorizator: func(data interface{}, c *gin.Context) bool {
			if _, ok := data.(model.UserClaims); ok {
				return true
			}
			return false
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.JSON(code, gin.H{
				"code":    code,
				"message": message,
			})
		},
		TokenLookup:   "header: Authorization, query: token, cookie: jwt",
		TokenHeadName: "Bearer",
		TimeFunc:      time.Now,
	})
	if err != nil {
		log.Error(err.Error())
	}
	return jwtMiddleware
}

func LoginResponse(c *gin.Context, code int, token string, expire time.Time) {
	c.JSON(http.StatusOK, gin.H{
		"code": code,
		"data": map[string]interface{}{
			"token":  token,
			"expire": expire,
		},
		"message": "success",
	})
}
func Authenticator(c *gin.Context, LoginType int) (interface{}, error) {
	var loginDto dto.LoginDto
	if err := dto.Bind(c, &loginDto); err != nil {
		return "", err
	}

	// login log
	loginLogDto := dto.LoginLogDto{
		Client:           c.Request.UserAgent(),
		Ip:               c.ClientIP(),
		IpLocation:       "", //TODO
		LoginStatus:      1,
		OperationContent: fmt.Sprintf("%s %s", c.Request.Method, c.Request.RequestURI),
	}
	if LoginType == account.LoginLdap { // LDAP login
		ok, u := accountService.VerifyAndReturnLdapUserInfo(loginDto)
		if ok {
			loginLogDto.UserId = u.Id
			loginLogDto.Platform = "LDAP Login"
			loginLogDto.LoginResult = "LDAP Login Success"
			_ = accountService.InsertLoginLog(&loginLogDto)
			return model.UserClaims{
				Id:   u.Id,
				Name: u.Username,
			}, nil
		}
		return nil, jwt.ErrFailedAuthentication
	}
	ok, err, u := accountService.VerifyAndReturnUserInfo(loginDto) // Standard login
	if ok {
		loginLogDto.UserId = u.Id
		loginLogDto.Platform = "Standard Login"
		loginLogDto.LoginResult = "Standard Login Success"
		_ = accountService.InsertLoginLog(&loginLogDto)

		return model.UserClaims{
			Id:   u.Id,
			Name: u.Username,
		}, nil
	}
	//return nil, jwt.ErrFailedAuthentication
	return nil, err
}

func AuthenticatorOAuth(c *gin.Context) (interface{}, error) {
	oauthDto := &dto.LoginOAuthDto{}
	if err := dto.Bind(c, &oauthDto); err != nil {
		log.Error(err.Error())
		return "", err
	}
	//TODO 支持微信、钉钉、QQ等登陆
	//login log
	loginLogDto := dto.LoginLogDto{
		Client:           c.Request.UserAgent(),
		Ip:               c.ClientIP(),
		IpLocation:       "", //TODO
		LoginStatus:      1,
		OperationContent: fmt.Sprintf("%s %s", c.Request.Method, c.Request.RequestURI),
	}
	if oauthDto.Type == account.OAuthDingTalk { //dingtalk
		user, err := accountService.VerifyDTAndReturnUserInfo(oauthDto.Code)
		if err != nil || user.Id < 1 {
			return "", err
		}
		loginLogDto.LoginResult = "DingTalk login success"
		loginLogDto.UserId = user.Id
		loginLogDto.Platform = "DingTalk Login"
		_ = accountService.InsertLoginLog(&loginLogDto)
		return model.UserClaims{
			Id:   user.Id,
			Name: user.Username,
		}, nil
	}
	return "", errors.New("")
}
