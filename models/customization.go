package models

import (
	"encoding/json"
	"github.com/openneo/neopia/amfphp"
)

type CustomizationService struct {
	getViewerData amfphp.RemoteMethod
}

func NewCustomizationService(gateway amfphp.RemoteGateway) CustomizationService {
	return CustomizationService{getViewerData: gateway.Service("CustomPetService").Method("getViewerData", customizationResponseIsPresent)}
}

func customizationResponseIsPresent(body []byte) bool {
	return len(body) > 0
}

func (s CustomizationService) GetCustomization(petName string) (Customization, bool, error) {
	// The AMFPHP JSON interface is a bit silly: because PHP maps are
	// implemented as arrays, empty maps look like empty lists when JSONified.
	// So, we need to explicitly try to parse the possibly-empty fields as
	// maps, and explicitly fall back to an empty map if it fails.

	var cr customizationResponse
	present, err := s.getViewerData.Call(&cr, petName)
	if !(present && err == nil) {
		return Customization{}, present, err
	}

	cp := CustomPet{
		Name:          cr.CustomPet.Name,
		Owner:         cr.CustomPet.Owner,
		Slot:          cr.CustomPet.Slot,
		Scale:         cr.CustomPet.Scale,
		Muted:         cr.CustomPet.Muted,
		BodyId:        cr.CustomPet.BodyId,
		SpeciesId:     cr.CustomPet.SpeciesId,
		ColorId:       cr.CustomPet.ColorId,
		BiologyByZone: cr.CustomPet.BiologyByZone,
	}
	err = json.Unmarshal(cr.CustomPet.EquippedByZone, &cp.EquippedByZone)
	if err != nil {
		cp.EquippedByZone = make(map[string]Equipped)
	}

	c := Customization{CustomPet: cp}
	err = json.Unmarshal(cr.ClosetItems, &c.ClosetItems)
	if err != nil {
		c.ClosetItems = make(map[string]ClosetItem)
	}
	err = json.Unmarshal(cr.ObjectInfoRegistry, &c.ObjectInfoRegistry)
	if err != nil {
		c.ObjectInfoRegistry = make(map[string]ObjectInfo)
	}
	err = json.Unmarshal(cr.ObjectAssetRegistry, &c.ObjectAssetRegistry)
	if err != nil {
		c.ObjectAssetRegistry = make(map[string]ObjectAsset)
	}

	return c, true, nil
}

type Customization struct {
	CustomPet           CustomPet              `json:"custom_pet"`
	ClosetItems         map[string]ClosetItem  `json:"closet_items"`
	ObjectInfoRegistry  map[string]ObjectInfo  `json:"object_info_registry"`
	ObjectAssetRegistry map[string]ObjectAsset `json:"object_asset_registry"`
}

type customizationResponse struct {
	CustomPet           customPetResponse `json:"custom_pet"`
	ClosetItems         json.RawMessage   `json:"closet_items"`
	ObjectInfoRegistry  json.RawMessage   `json:"object_info_registry"`
	ObjectAssetRegistry json.RawMessage   `json:"object_asset_registry"`
}

type CustomPet struct {
	Name           string              `json:"name"`
	Owner          string              `json:"owner"`
	Slot           int                 `json:"slot"`
	Scale          float64             `json:"scale"`
	Muted          bool                `json:"muted"`
	BodyId         int                 `json:"body_id"`
	SpeciesId      int                 `json:"species_id"`
	ColorId        int                 `json:"color_id"`
	BiologyByZone  map[string]Biology  `json:"biology_by_zone"`
	EquippedByZone map[string]Equipped `json:"equipped_by_zone"`
}

type customPetResponse struct {
	Name           string             `json:"name"`
	Owner          string             `json:"owner"`
	Slot           int                `json:"slot"`
	Scale          float64            `json:"scale"`
	Muted          bool               `json:"muted"`
	BodyId         int                `json:"body_id"`
	SpeciesId      int                `json:"species_id"`
	ColorId        int                `json:"color_id"`
	BiologyByZone  map[string]Biology `json:"biology_by_zone"`
	EquippedByZone json.RawMessage    `json:"equipped_by_zone"`
}

type Biology struct {
	PartId        int    `json:"part_id"`
	ZoneId        int    `json:"zone_id"`
	AssetUrl      string `json:"asset_url"`
	ZonesRestrict string `json:"zones_restrict"`
}

type Equipped struct {
	AssetId        int `json:"asset_id"`
	ZoneId         int `json:"zone_id"`
	ClosetObjectId int `json:"closet_obj_id"`
}

type ClosetItem struct {
	ClosetObjectId int    `json:"closet_obj_id"`
	ObjectInfoId   int    `json:"obj_info_id"`
	AppliedTo      string `json:"applied_to"`
	IsWishlist     bool   `json:"is_wishlist"`
	Expiration     string `json:"expiration"`
}

type ObjectInfo struct {
	Id             int            `json:"obj_info_id"`
	AssetIdsByZone map[string]int `json:"assets_by_zone"`
	ZonesRestrict  string         `json:"zones_restrict"`
	IsCompatible   bool           `json:"is_compatible"`
	IsPaid         bool           `json:"is_paid"`
	ThumbnailUrl   string         `json:"thumbnail_url"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	Type           string         `json:"type"`
	Rarity         string         `json:"rarity"`
	RarityIndex    int            `json:"rarity_index"`
	Price          int            `json:"price"`
	WeightLbs      int            `json:"weight_lbs"`
	SpeciesSupport []int          `json:"species_support"`
}

type ObjectAsset struct {
	AssetId      int    `json:"asset_id"`
	ZoneId       int    `json:"zone_id"`
	AssetUrl     string `json:"asset_url"`
	ObjectInfoId int    `json:"obj_info_id"`
}
