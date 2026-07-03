package user

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"pandabk_coin/internal/apierror"
)

type Handler struct {
	Service   *Service
	JWTSecret string
}

type createRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Active   *bool  `json:"active"`
}

type updateRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password"`
	Active   *bool  `json:"active"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Create godoc
// @Summary      Create a user
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        user  body      createRequest  true  "User to create"
// @Success      201   {object}  User
// @Failure      400   {object}  apierror.Body
// @Failure      409   {object}  apierror.Body
// @Failure      500   {object}  apierror.Body
// @Router       /users [post]
func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierror.Respond(c, apierror.BadRequest("validation_error", err.Error()))
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	u, err := h.Service.Create(CreateInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Active:   active,
	})
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			apierror.Respond(c, apierror.Conflict("email_taken", "email already in use"))
			return
		}
		apierror.Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, u)
}

// List godoc
// @Summary      List users
// @Tags         users
// @Produce      json
// @Success      200  {array}   User
// @Failure      500  {object}  apierror.Body
// @Router       /users [get]
func (h *Handler) List(c *gin.Context) {
	users, err := h.Service.List()
	if err != nil {
		apierror.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, users)
}

// Get godoc
// @Summary      Get a user by id
// @Tags         users
// @Produce      json
// @Param        id   path      int  true  "User ID"
// @Success      200  {object}  User
// @Failure      400  {object}  apierror.Body
// @Failure      404  {object}  apierror.Body
// @Router       /users/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierror.Respond(c, apierror.BadRequest("invalid_id", "invalid id"))
		return
	}

	u, err := h.Service.GetByID(id)
	if err != nil {
		apierror.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, u)
}

// Update godoc
// @Summary      Update a user
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id    path      int            true  "User ID"
// @Param        user  body      updateRequest  true  "Fields to update"
// @Success      200   {object}  User
// @Failure      400   {object}  apierror.Body
// @Failure      404   {object}  apierror.Body
// @Failure      409   {object}  apierror.Body
// @Failure      500   {object}  apierror.Body
// @Router       /users/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierror.Respond(c, apierror.BadRequest("invalid_id", "invalid id"))
		return
	}

	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierror.Respond(c, apierror.BadRequest("validation_error", err.Error()))
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	u, err := h.Service.Update(id, UpdateInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Active:   active,
	})
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			apierror.Respond(c, apierror.Conflict("email_taken", "email already in use"))
			return
		}
		apierror.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, u)
}

// Delete godoc
// @Summary      Delete a user
// @Tags         users
// @Param        id  path  int  true  "User ID"
// @Success      204
// @Failure      400  {object}  apierror.Body
// @Router       /users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierror.Respond(c, apierror.BadRequest("invalid_id", "invalid id"))
		return
	}

	if err := h.Service.Delete(id); err != nil {
		apierror.Respond(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// Login godoc
// @Summary      Login
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        credentials  body      loginRequest  true  "Login credentials"
// @Success      200          {object}  loginResponse
// @Failure      400          {object}  apierror.Body
// @Failure      401          {object}  apierror.Body
// @Failure      500          {object}  apierror.Body
// @Router       /login [post]
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierror.Respond(c, apierror.BadRequest("validation_error", err.Error()))
		return
	}

	u, err := h.Service.Authenticate(req.Email, req.Password)
	if err != nil {
		apierror.Respond(c, apierror.Unauthorized("invalid_credentials", "invalid credentials"))
		return
	}

	token, err := GenerateToken(h.JWTSecret, u.ID)
	if err != nil {
		apierror.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// Me godoc
// @Summary      Get the authenticated user
// @Description  Reference example for protecting a route with AuthRequired.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  User
// @Failure      401  {object}  apierror.Body
// @Failure      404  {object}  apierror.Body
// @Router       /me [get]
func (h *Handler) Me(c *gin.Context) {
	rawID, exists := c.Get(ContextUserIDKey)
	if !exists {
		apierror.Respond(c, apierror.Unauthorized("unauthorized", "unauthorized"))
		return
	}

	u, err := h.Service.GetByID(rawID.(uint))
	if err != nil {
		apierror.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, u)
}

func parseID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
