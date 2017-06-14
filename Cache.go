package arn

// ListOfIDs ...
type ListOfIDs struct {
	IDList []string `json:"idList"`
}

// GetListOfIDs ...
func GetListOfIDs(table string, id string) (*ListOfIDs, error) {
	cache := &ListOfIDs{}
	err := DB.GetObject(table, id, cache)
	return cache, err
}

// GetAiringAnimeCached ...
func GetAiringAnimeCached() ([]*Anime, error) {
	cache, err := GetListOfIDs("Cache", "airing anime")

	if err != nil {
		return nil, err
	}

	list, err := DB.GetMany("Anime", cache.IDList)
	return list.([]*Anime), err
}

// GetActiveUsersCached ...
func GetActiveUsersCached() ([]*User, error) {
	cache, err := GetListOfIDs("Cache", "active users")

	if err != nil {
		return nil, err
	}

	list, err := DB.GetMany("User", cache.IDList)
	return list.([]*User), err
}