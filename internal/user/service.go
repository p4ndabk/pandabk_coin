package user

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrEmailTaken = errors.New("email already in use")

type Service struct {
	DB *gorm.DB
}

type CreateInput struct {
	Name     string
	Email    string
	Password string
	Active   bool
}

type UpdateInput struct {
	Name     string
	Email    string
	Password string
	Active   bool
}

func (s *Service) Create(input CreateInput) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &User{
		Name:     input.Name,
		Email:    input.Email,
		Password: string(hashed),
		Active:   input.Active,
	}

	if err := s.DB.Create(u).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	return u, nil
}

func (s *Service) List() ([]User, error) {
	var users []User
	if err := s.DB.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Service) GetByID(id uint) (*User, error) {
	var u User
	if err := s.DB.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Service) Update(id uint, input UpdateInput) (*User, error) {
	u, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	u.Name = input.Name
	u.Email = input.Email
	u.Active = input.Active

	if input.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		u.Password = string(hashed)
	}

	if err := s.DB.Save(u).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	return u, nil
}

func (s *Service) Delete(id uint) error {
	return s.DB.Delete(&User{}, id).Error
}

func (s *Service) Authenticate(email, password string) (*User, error) {
	var u User
	if err := s.DB.Where("email = ?", email).First(&u).Error; err != nil {
		return nil, ErrInvalidCredentials
	}

	if !u.Active {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return &u, nil
}
