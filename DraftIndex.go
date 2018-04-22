package arn

// DraftIndex has references to unpublished drafts a user created.
type DraftIndex struct {
	UserID       string `json:"userId"`
	GroupID      string `json:"groupId"`
	SoundTrackID string `json:"soundTrackId"`
	CompanyID    string `json:"companyId"`
	QuoteID      string `json:"quoteId"`
	CharacterID  string `json:"characterId"`
	AnimeID      string `json:"animeId"`
	AMVID        string `json:"amvId"`
}

// NewDraftIndex ...
func NewDraftIndex(userID string) *DraftIndex {
	return &DraftIndex{
		UserID: userID,
	}
}

// GetDraftIndex ...
func GetDraftIndex(id string) (*DraftIndex, error) {
	obj, err := DB.Get("DraftIndex", id)

	if err != nil {
		return nil, err
	}

	return obj.(*DraftIndex), nil
}
