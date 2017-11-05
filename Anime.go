package arn

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerogo/nano"
	"github.com/animenotifier/arn/validator"
	"github.com/animenotifier/twist"

	"github.com/animenotifier/kitsu"
	"github.com/animenotifier/shoboi"
	"github.com/fatih/color"
)

// Anime ...
type Anime struct {
	ID            string           `json:"id"`
	Type          string           `json:"type"`
	Title         *AnimeTitle      `json:"title"`
	Image         *AnimeImageTypes `json:"image"`
	FirstChannel  string           `json:"firstChannel"`
	StartDate     string           `json:"startDate"`
	EndDate       string           `json:"endDate"`
	EpisodeCount  int              `json:"episodeCount"`
	EpisodeLength int              `json:"episodeLength"`
	Status        string           `json:"status"`
	NSFW          int              `json:"nsfw"`
	Rating        *AnimeRating     `json:"rating"`
	Popularity    *AnimePopularity `json:"popularity"`
	Summary       string           `json:"summary"`
	Trailers      []*ExternalMedia `json:"trailers"`
	Mappings      []*Mapping       `json:"mappings"`

	// Hashtag       string          `json:"hashtag"`
	// Source        string          `json:"source"`

	// PageGenerated string          `json:"pageGenerated"`
	// AnilistEdited uint64          `json:"anilistEdited"`
	// Genres        []string        `json:"genres"`
	// Tracks        *AnimeTrackList `json:"tracks"`
	// Links         []AnimeLink     `json:"links"`
	// Studios       []AnimeStudio   `json:"studios"`
	// Relations     []AnimeRelation `json:"relations"`
	// Created       string          `json:"created"`
	// CreatedBy     string          `json:"createdBy"`

	// episodes   *AnimeEpisodes
	// relations  *AnimeRelations
	// characters *AnimeCharacters
}

// AnimeImageTypes ...
type AnimeImageTypes struct {
	Tiny     string `json:"tiny"`
	Small    string `json:"small"`
	Large    string `json:"large"`
	Original string `json:"original"`
}

// GetAnime ...
func GetAnime(id string) (*Anime, error) {
	obj, err := DB.Get("Anime", id)

	if err != nil {
		return nil, err
	}

	return obj.(*Anime), nil
}

// Characters ...
func (anime *Anime) Characters() *AnimeCharacters {
	characters, _ := GetAnimeCharacters(anime.ID)

	if characters != nil {
		// TODO: Sort by role in sync-characters job
		// Sort by role
		sort.Slice(characters.Items, func(i, j int) bool {
			// A little trick because "main" < "supporting"
			return characters.Items[i].Role < characters.Items[j].Role
		})
	}

	return characters
}

// Relations ...
func (anime *Anime) Relations() *AnimeRelations {
	relations, _ := GetAnimeRelations(anime.ID)
	return relations
}

// Link returns the URI to the anime page.
func (anime *Anime) Link() string {
	return "/anime/" + anime.ID
}

// PrettyJSON ...
func (anime *Anime) PrettyJSON() (string, error) {
	data, err := json.MarshalIndent(anime, "", "    ")
	return string(data), err
}

// AddMapping adds the ID of an external site to the anime.
func (anime *Anime) AddMapping(serviceName string, serviceID string, userID string) {
	// Is the ID valid?
	if serviceID == "" {
		return
	}

	// If it already exists we don't need to add it
	for _, external := range anime.Mappings {
		if external.Service == serviceName && external.ServiceID == serviceID {
			return
		}
	}

	// Add the mapping
	anime.Mappings = append(anime.Mappings, &Mapping{
		Service:   serviceName,
		ServiceID: serviceID,
		Created:   DateTimeUTC(),
		CreatedBy: userID,
	})

	// Add the references
	switch serviceName {
	case "shoboi/anime":
		go anime.RefreshEpisodes()

	case "anilist/anime":
		DB.Set("AniListToAnime", serviceID, &AniListToAnime{
			AnimeID:   anime.ID,
			ServiceID: serviceID,
			Edited:    DateTimeUTC(),
			EditedBy:  userID,
		})

	case "myanimelist/anime":
		DB.Set("MyAnimeListToAnime", serviceID, &MyAnimeListToAnime{
			AnimeID:   anime.ID,
			ServiceID: serviceID,
			Edited:    DateTimeUTC(),
			EditedBy:  userID,
		})
	}
}

// GetMapping returns the external ID for the given service.
func (anime *Anime) GetMapping(name string) string {
	for _, external := range anime.Mappings {
		if external.Service == name {
			return external.ServiceID
		}
	}

	return ""
}

// RemoveMapping removes all mappings with the given service name and ID.
func (anime *Anime) RemoveMapping(name string, id string) bool {
	switch name {
	case "shoboi/anime":
		eps := anime.Episodes()

		if eps != nil {
			eps.Items = eps.Items[:0]
			eps.Save()
		}
	case "anilist/anime":
		DB.Delete("AniListToAnime", id)
	case "myanimelist/anime":
		DB.Delete("MyAnimeListToAnime", id)
	}

	for index, external := range anime.Mappings {
		if external.Service == name && external.ServiceID == id {
			anime.Mappings = append(anime.Mappings[:index], anime.Mappings[index+1:]...)
			return true
		}
	}

	return false
}

// Episodes returns the anime episodes wrapper.
func (anime *Anime) Episodes() *AnimeEpisodes {
	record, _ := DB.Get("AnimeEpisodes", anime.ID)
	return record.(*AnimeEpisodes)
}

// UsersWatchingOrPlanned returns a list of users who are watching the anime right now.
func (anime *Anime) UsersWatchingOrPlanned() []*User {
	users := FilterUsers(func(user *User) bool {
		item := user.AnimeList().Find(anime.ID)

		if item == nil {
			return false
		}

		return item.Status == AnimeListStatusWatching || item.Status == AnimeListStatusPlanned
	})

	return users
}

// RefreshEpisodes will refresh the episode data.
func (anime *Anime) RefreshEpisodes() error {
	// Fetch episodes
	episodes := anime.Episodes()

	if episodes == nil {
		episodes = &AnimeEpisodes{
			AnimeID: anime.ID,
			Items:   []*AnimeEpisode{},
		}
	}

	// Save number of available episodes for comparison later
	oldAvailableCount := episodes.AvailableCount()

	// Create blank episode templates
	episodes.Items = make([]*AnimeEpisode, anime.EpisodeCount, anime.EpisodeCount)

	for i := 0; i < len(episodes.Items); i++ {
		episodes.Items[i] = NewAnimeEpisode()
	}

	// Shoboi
	shoboiEpisodes, err := anime.ShoboiEpisodes()

	if err != nil {
		color.Red(err.Error())
	}

	episodes.Merge(shoboiEpisodes)

	// AnimeTwist
	twistEpisodes, err := anime.TwistEpisodes()

	if err != nil {
		color.Red(err.Error())
	}

	episodes.Merge(twistEpisodes)

	// Count number of available episodes
	newAvailableCount := episodes.AvailableCount()

	if anime.Status != "finished" && newAvailableCount > oldAvailableCount {
		notification := &Notification{
			Title:   anime.Title.Canonical,
			Message: "Episode " + strconv.Itoa(newAvailableCount) + " has been released!",
			Icon:    anime.Image.Small,
			Link:    "https://notify.moe" + anime.Link(),
		}

		// New episodes have been released.
		// Notify all users who are watching the anime.
		go func() {
			for _, user := range anime.UsersWatchingOrPlanned() {
				user.SendNotification(notification)
			}
		}()
	}

	// Number remaining episodes
	startNumber := 0

	for _, episode := range episodes.Items {
		if episode.Number != -1 {
			startNumber = episode.Number
			continue
		}

		startNumber++
		episode.Number = startNumber
	}

	// Guess airing dates
	oneWeek := 7 * 24 * time.Hour
	lastAiringDate := ""
	timeDifference := oneWeek

	for _, episode := range episodes.Items {
		if validator.IsValidDate(episode.AiringDate.Start) {
			if lastAiringDate != "" {
				a, _ := time.Parse(time.RFC3339, lastAiringDate)
				b, _ := time.Parse(time.RFC3339, episode.AiringDate.Start)
				timeDifference = b.Sub(a)

				// Cap time difference at one week
				if timeDifference > oneWeek {
					timeDifference = oneWeek
				}
			}

			lastAiringDate = episode.AiringDate.Start
			continue
		}

		// Add 1 week to the last known airing date
		nextAiringDate, _ := time.Parse(time.RFC3339, lastAiringDate)
		nextAiringDate = nextAiringDate.Add(timeDifference)

		// Guess start and end time
		episode.AiringDate.Start = nextAiringDate.Format(time.RFC3339)
		episode.AiringDate.End = nextAiringDate.Add(30 * time.Minute).Format(time.RFC3339)

		// Set this date as the new last known airing date
		lastAiringDate = episode.AiringDate.Start
	}

	episodes.Save()

	return nil
}

// ShoboiEpisodes returns a slice of episode info from cal.syoboi.jp.
func (anime *Anime) ShoboiEpisodes() ([]*AnimeEpisode, error) {
	shoboiID := anime.GetMapping("shoboi/anime")

	if shoboiID == "" {
		return nil, errors.New("Missing shoboi/anime mapping")
	}

	shoboiAnime, err := shoboi.GetAnime(shoboiID)

	if err != nil {
		return nil, err
	}

	arnEpisodes := []*AnimeEpisode{}
	shoboiEpisodes := shoboiAnime.Episodes()

	for _, shoboiEpisode := range shoboiEpisodes {
		episode := NewAnimeEpisode()
		episode.Number = shoboiEpisode.Number
		episode.Title = &EpisodeTitle{
			Japanese: shoboiEpisode.TitleJapanese,
		}

		// Try to get airing date
		airingDate := shoboiEpisode.AiringDate

		if airingDate != nil {
			episode.AiringDate = &AnimeAiringDate{
				Start: airingDate.Start,
				End:   airingDate.End,
			}
		} else {
			episode.AiringDate = &AnimeAiringDate{
				Start: "",
				End:   "",
			}
		}

		arnEpisodes = append(arnEpisodes, episode)
	}

	return arnEpisodes, nil
}

// TwistEpisodes returns a slice of episode info from twist.moe.
func (anime *Anime) TwistEpisodes() ([]*AnimeEpisode, error) {
	idList, err := GetIDList("animetwist index")

	if err != nil {
		return nil, err
	}

	// Does the index contain the ID?
	found := false

	for _, id := range idList {
		if id == anime.ID {
			found = true
			break
		}
	}

	// If the ID is not the index we don't need to query the feed
	if !found {
		return nil, errors.New("Not available in twist.moe anime index")
	}

	// Get twist.moe feed
	feed, err := twist.GetFeedByKitsuID(anime.ID)

	if err != nil {
		return nil, err
	}

	episodes := feed.Episodes

	// Sort by episode number
	sort.Slice(episodes, func(a, b int) bool {
		return episodes[a].Number < episodes[b].Number
	})

	arnEpisodes := []*AnimeEpisode{}

	for _, episode := range episodes {
		arnEpisode := NewAnimeEpisode()
		arnEpisode.Number = episode.Number
		arnEpisode.Links = map[string]string{
			"twist.moe": strings.Replace(episode.Link, "https://test.twist.moe/", "https://twist.moe/", 1),
		}

		arnEpisodes = append(arnEpisodes, arnEpisode)
	}

	return arnEpisodes, nil
}

// UpcomingEpisodes ...
func (anime *Anime) UpcomingEpisodes() []*UpcomingEpisode {
	var upcomingEpisodes []*UpcomingEpisode

	now := time.Now().UTC().Format(time.RFC3339)

	for _, episode := range anime.Episodes().Items {
		if episode.AiringDate.Start > now && validator.IsValidDate(episode.AiringDate.Start) {
			upcomingEpisodes = append(upcomingEpisodes, &UpcomingEpisode{
				Anime:   anime,
				Episode: episode,
			})
		}
	}

	return upcomingEpisodes
}

// UpcomingEpisode ...
func (anime *Anime) UpcomingEpisode() *UpcomingEpisode {
	now := time.Now().UTC().Format(time.RFC3339)

	for _, episode := range anime.Episodes().Items {
		if episode.AiringDate.Start > now && validator.IsValidDate(episode.AiringDate.Start) {
			return &UpcomingEpisode{
				Anime:   anime,
				Episode: episode,
			}
		}
	}

	return nil
}

// EpisodeCountString ...
func (anime *Anime) EpisodeCountString() string {
	if anime.EpisodeCount == 0 {
		return "?"
	}

	return strconv.Itoa(anime.EpisodeCount)
}

// TypeHumanReadable ...
func (anime *Anime) TypeHumanReadable() string {
	switch anime.Type {
	case "tv":
		return "TV"
	case "movie":
		return "Movie"
	case "ova":
		return "OVA"
	case "ona":
		return "ONA"
	case "special":
		return "Special"
	default:
		return anime.Type
	}
}

// StatusHumanReadable ...
func (anime *Anime) StatusHumanReadable() string {
	switch anime.Status {
	case "finished":
		return "Finished"
	case "current":
		return "Airing"
	case "upcoming":
		return "Upcoming"
	case "unannounced":
		return "Unannounced"
	case "tba":
		return "To be announced"
	default:
		return anime.Status
	}
}

// EpisodeByNumber returns the episode with the given number.
func (anime *Anime) EpisodeByNumber(number int) *AnimeEpisode {
	for _, episode := range anime.Episodes().Items {
		if number == episode.Number {
			return episode
		}
	}

	return nil
}

// RefreshAnimeCharacters ...
func (anime *Anime) RefreshAnimeCharacters() (*AnimeCharacters, error) {
	resp, err := kitsu.GetAnimeCharactersForAnime(anime.ID)

	if err != nil {
		return nil, err
	}

	animeCharacters := &AnimeCharacters{
		AnimeID: anime.ID,
		Items:   []*AnimeCharacter{},
	}

	for _, incl := range resp.Included {
		if incl.Type != "animeCharacters" {
			continue
		}

		role := incl.Attributes["role"].(string)
		characterID := incl.Relationships.Character.Data.ID

		animeCharacter := &AnimeCharacter{
			CharacterID: characterID,
			Role:        role,
		}

		animeCharacters.Items = append(animeCharacters.Items, animeCharacter)
	}

	animeCharacters.Save()

	return animeCharacters, nil
}

// StreamAnime returns a stream of all anime.
func StreamAnime() chan *Anime {
	channel := make(chan *Anime, nano.ChannelBufferSize)

	go func() {
		for obj := range DB.All("Anime") {
			channel <- obj.(*Anime)
		}

		close(channel)
	}()

	return channel
}

// AllAnime returns a slice of all anime.
func AllAnime() ([]*Anime, error) {
	var all []*Anime

	stream := StreamAnime()

	for obj := range stream {
		all = append(all, obj)
	}

	return all, nil
}

// FilterAnime filters all anime by a custom function.
func FilterAnime(filter func(*Anime) bool) []*Anime {
	var filtered []*Anime

	channel := DB.All("Anime")

	for obj := range channel {
		realObject := obj.(*Anime)

		if filter(realObject) {
			filtered = append(filtered, realObject)
		}
	}

	return filtered
}

// GetAiringAnime ...
func GetAiringAnime() []*Anime {
	beforeTime := time.Now().Add(-6 * 30 * 24 * time.Hour)
	beforeTimeString := beforeTime.Format(time.RFC3339)

	return FilterAnime(func(anime *Anime) bool {
		if (anime.Type != "tv" && anime.Type != "movie") || anime.NSFW == 1 || anime.StartDate < beforeTimeString {
			return false
		}

		if anime.Popularity.Total() == 0 {
			return false
		}

		// return anime.UpcomingEpisode() != nil || anime.Status == "upcoming"
		return anime.Status == "current" || anime.Status == "upcoming"
	})
}
