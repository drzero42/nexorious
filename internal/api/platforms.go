package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)

// PlatformsHandler handles platform and storefront endpoints.
type PlatformsHandler struct {
	db *bun.DB
}

// NewPlatformsHandler returns a new PlatformsHandler.
func NewPlatformsHandler(db *bun.DB) *PlatformsHandler {
	// Register the m2m join model so bun can resolve the Platform<->Storefront
	// many-to-many relation via Relation("Storefronts").
	db.RegisterModel((*models.PlatformStorefront)(nil))
	return &PlatformsHandler{db: db}
}

// HandleListPlatforms handles GET /api/platforms.
// Returns all platforms with their storefronts, ordered by display_name.
func (h *PlatformsHandler) HandleListPlatforms(c *echo.Context) error {
	var platforms []models.Platform
	err := h.db.NewSelect().
		Model(&platforms).
		Relation("Storefronts").
		OrderExpr("\"platform\".display_name ASC").
		Scan(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list platforms")
	}
	resp := make([]platformResponse, len(platforms))
	for i, p := range platforms {
		resp[i] = toPlatformResponse(p)
	}
	n := len(resp)
	return c.JSON(http.StatusOK, map[string]any{
		"platforms": resp,
		"total":     n,
		"page":      1,
		"per_page":  n,
		"pages":     1,
	})
}

// simpleItem is a minimal name/display_name pair used by simple-list endpoints.
type simpleItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// storefrontResponse is the API response DTO for a storefront, including icon_url.
type storefrontResponse struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Icon        *string `json:"icon"`
	BaseURL     *string `json:"base_url"`
	IconURL     *string `json:"icon_url"`
}

// platformResponse is the API response DTO for a platform, including icon_url.
type platformResponse struct {
	Name              string               `json:"name"`
	DisplayName       string               `json:"display_name"`
	Icon              *string              `json:"icon"`
	IgdbPlatformID    *int32               `json:"igdb_platform_id"`
	DefaultStorefront *string              `json:"default_storefront"`
	Storefronts       []storefrontResponse `json:"storefronts,omitempty"`
	IconURL           *string              `json:"icon_url"`
}

// iconURL constructs the logo URL for a platform or storefront icon.
// Returns nil if icon is nil.
func iconURL(category, name string, icon *string) *string {
	if icon == nil {
		return nil
	}
	u := fmt.Sprintf("/logos/%s/%s/%s", category, name, *icon)
	return &u
}

// toStorefrontResponse converts a models.Storefront to a storefrontResponse DTO.
func toStorefrontResponse(sf models.Storefront) storefrontResponse {
	return storefrontResponse{
		Name:        sf.Name,
		DisplayName: sf.DisplayName,
		Icon:        sf.Icon,
		BaseURL:     sf.BaseUrl,
		IconURL:     iconURL("storefronts", sf.Name, sf.Icon),
	}
}

// toPlatformResponse converts a models.Platform to a platformResponse DTO.
func toPlatformResponse(p models.Platform) platformResponse {
	resp := platformResponse{
		Name:              p.Name,
		DisplayName:       p.DisplayName,
		Icon:              p.Icon,
		IgdbPlatformID:    p.IgdbPlatformID,
		DefaultStorefront: p.DefaultStorefront,
		IconURL:           iconURL("platforms", p.Name, p.Icon),
	}
	for _, sf := range p.Storefronts {
		resp.Storefronts = append(resp.Storefronts, toStorefrontResponse(sf))
	}
	return resp
}

// HandleSimpleList handles GET /api/platforms/simple-list.
// Returns only name and display_name for each platform.
func (h *PlatformsHandler) HandleSimpleList(c *echo.Context) error {
	var items []simpleItem
	err := h.db.NewSelect().
		TableExpr("platforms").
		ColumnExpr("name, display_name").
		OrderExpr("display_name ASC").
		Scan(context.Background(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list platforms")
	}
	return c.JSON(http.StatusOK, items)
}

// HandleGetPlatform handles GET /api/platforms/:platform.
// Returns the platform (with storefronts) or 404.
func (h *PlatformsHandler) HandleGetPlatform(c *echo.Context) error {
	name := c.Param("platform")
	var platform models.Platform
	err := h.db.NewSelect().
		Model(&platform).
		Where("\"platform\".name = ?", name).
		Relation("Storefronts").
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "platform not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get platform")
	}
	return c.JSON(http.StatusOK, toPlatformResponse(platform))
}

// HandlePlatformStorefronts handles GET /api/platforms/:platform/storefronts.
// Returns the storefronts associated with the platform, or 404 if platform not found.
func (h *PlatformsHandler) HandlePlatformStorefronts(c *echo.Context) error {
	name := c.Param("platform")

	// Verify platform exists.
	exists, err := h.db.NewSelect().
		TableExpr("platforms").
		Where("name = ?", name).
		Exists(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check platform")
	}
	if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "platform not found")
	}

	var storefronts []models.Storefront
	err = h.db.NewSelect().
		Model(&storefronts).
		Join("JOIN platform_storefronts ps ON ps.storefront = \"storefront\".name").
		Where("ps.platform = ?", name).
		OrderExpr("\"storefront\".display_name ASC").
		Scan(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list storefronts")
	}
	resp := make([]storefrontResponse, len(storefronts))
	for i, sf := range storefronts {
		resp[i] = toStorefrontResponse(sf)
	}
	return c.JSON(http.StatusOK, resp)
}

// defaultStorefrontResponse is the response for GET /api/platforms/:platform/default-storefront.
type defaultStorefrontResponse struct {
	Platform            string              `json:"platform"`
	PlatformDisplayName string              `json:"platform_display_name"`
	DefaultStorefront   *storefrontResponse `json:"default_storefront"`
}

// HandleDefaultStorefront handles GET /api/platforms/:platform/default-storefront.
func (h *PlatformsHandler) HandleDefaultStorefront(c *echo.Context) error {
	name := c.Param("platform")

	var platform models.Platform
	err := h.db.NewSelect().
		Model(&platform).
		Where("\"platform\".name = ?", name).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "platform not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get platform")
	}

	resp := defaultStorefrontResponse{
		Platform:            platform.Name,
		PlatformDisplayName: platform.DisplayName,
	}

	if platform.DefaultStorefront != nil {
		var sf models.Storefront
		err = h.db.NewSelect().
			Model(&sf).
			Where("\"storefront\".name = ?", *platform.DefaultStorefront).
			Scan(context.Background())
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get default storefront")
		}
		if err == nil {
			sfResp := toStorefrontResponse(sf)
			resp.DefaultStorefront = &sfResp
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleListStorefronts handles GET /api/platforms/storefronts.
// Returns all storefronts ordered by display_name.
func (h *PlatformsHandler) HandleListStorefronts(c *echo.Context) error {
	var storefronts []models.Storefront
	err := h.db.NewSelect().
		Model(&storefronts).
		OrderExpr("\"storefront\".display_name ASC").
		Scan(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list storefronts")
	}
	resp := make([]storefrontResponse, len(storefronts))
	for i, sf := range storefronts {
		resp[i] = toStorefrontResponse(sf)
	}
	n := len(resp)
	return c.JSON(http.StatusOK, map[string]any{
		"storefronts": resp,
		"total":       n,
		"page":        1,
		"per_page":    n,
		"pages":       1,
	})
}

// HandleStorefrontSimpleList handles GET /api/platforms/storefronts/simple-list.
// Returns only name and display_name for each storefront.
func (h *PlatformsHandler) HandleStorefrontSimpleList(c *echo.Context) error {
	var items []simpleItem
	err := h.db.NewSelect().
		TableExpr("storefronts").
		ColumnExpr("name, display_name").
		OrderExpr("display_name ASC").
		Scan(context.Background(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list storefronts")
	}
	return c.JSON(http.StatusOK, items)
}

// HandleGetStorefront handles GET /api/platforms/storefronts/:storefront.
// Returns the storefront or 404.
func (h *PlatformsHandler) HandleGetStorefront(c *echo.Context) error {
	name := c.Param("storefront")
	var sf models.Storefront
	err := h.db.NewSelect().
		Model(&sf).
		Where("\"storefront\".name = ?", name).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "storefront not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get storefront")
	}
	return c.JSON(http.StatusOK, toStorefrontResponse(sf))
}
