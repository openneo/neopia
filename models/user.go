package models

import (
	"github.com/openneo/neopia/amfphp"
)

type UserService struct {
	getPets amfphp.RemoteMethod
}

func NewUserService(gateway amfphp.RemoteGateway) UserService {
	return UserService{getPets: gateway.Service("MobileService").Method("getPets", petsResponseIsPresent)}
}

func petsResponseIsPresent(body []byte) bool {
	return string(body) != "false"
}

func (s UserService) GetUser(name string) (User, bool, error) {
	var petResponses []petResponse
	present, err := s.getPets.Call(&petResponses, name)
	if !(present && err == nil) {
		return User{}, present, err
	}

	var user User
	user.Name = name
	for _, pr := range petResponses {
		user.PetNames = append(user.PetNames, pr.Name)
	}

	return user, true, nil
}

type petResponse struct {
	Name string
}

type User struct {
	Name     string
	PetNames []string
}
