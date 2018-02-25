package arn

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/animenotifier/arn/autocorrect"
	"github.com/animenotifier/arn/validator"
	"github.com/animenotifier/osu"
	gravatar "github.com/ungerik/go-gravatar"
)

var setNickMutex sync.Mutex
var setEmailMutex sync.Mutex

// User ...
type User struct {
	ID         string       `json:"id"`
	Nick       string       `json:"nick" editable:"true"`
	FirstName  string       `json:"firstName"`
	LastName   string       `json:"lastName"`
	Email      string       `json:"email"`
	Role       string       `json:"role"`
	Registered string       `json:"registered"`
	LastLogin  string       `json:"lastLogin"`
	LastSeen   string       `json:"lastSeen"`
	ProExpires string       `json:"proExpires"`
	Gender     string       `json:"gender"`
	Language   string       `json:"language"`
	Tagline    string       `json:"tagline" editable:"true"`
	Website    string       `json:"website" editable:"true"`
	IP         string       `json:"ip"`
	UserAgent  string       `json:"agent"`
	Balance    int          `json:"balance"`
	Avatar     UserAvatar   `json:"avatar"`
	AgeRange   UserAgeRange `json:"ageRange"`
	Location   Location     `json:"location"`
	Accounts   UserAccounts `json:"accounts"`
	Browser    UserBrowser  `json:"browser"`
	OS         UserOS       `json:"os"`
	Following  []string     `json:"following"`
}

// NewUser creates an empty user object with a unique ID.
func NewUser() *User {
	user := &User{
		ID: GenerateID("User"),

		// Avoid nil value fields
		Following: make([]string, 0),
	}

	return user
}

// RegisterUser registers a new user in the database and sets up all the required references.
func RegisterUser(user *User) {
	user.Registered = DateTimeUTC()
	user.LastLogin = user.Registered
	user.LastSeen = user.Registered

	// Save nick in NickToUser table
	DB.Set("NickToUser", user.Nick, &NickToUser{
		Nick:   user.Nick,
		UserID: user.ID,
	})

	// Save email in EmailToUser table
	DB.Set("EmailToUser", user.Email, &EmailToUser{
		Email:  user.Email,
		UserID: user.ID,
	})

	// Create default settings
	NewSettings(user).Save()

	// Add empty anime list
	DB.Set("AnimeList", user.ID, &AnimeList{
		UserID: user.ID,
		Items:  []*AnimeListItem{},
	})

	// Add empty inventory
	NewInventory(user.ID).Save()

	// Add draft index
	NewDraftIndex(user.ID).Save()

	// Add empty push subscriptions
	DB.Set("PushSubscriptions", user.ID, &PushSubscriptions{
		UserID: user.ID,
		Items:  []*PushSubscription{},
	})

	// Add empty follow list
	follows := &UserFollows{}
	follows.UserID = user.ID
	follows.Items = []string{}

	DB.Set("UserFollows", user.ID, follows)

	// Refresh avatar async
	go user.RefreshAvatar()
}

// SendNotification ...
func (user *User) SendNotification(notification *Notification) {
	// Don't ever send notifications in development mode
	if IsDevelopment() && user.ID != "4J6qpK1ve" {
		return
	}

	subs := user.PushSubscriptions()
	expired := []*PushSubscription{}

	for _, sub := range subs.Items {
		err := sub.SendNotification(notification)

		if err != nil {
			if err.Error() == "Subscription expired" {
				expired = append(expired, sub)
			}
		}
	}

	// RemoveQuote expired items
	if len(expired) > 0 {
		for _, sub := range expired {
			subs.Remove(sub.ID())
		}

		subs.Save()
	}
}

// RealName returns the real name of the user.
func (user *User) RealName() string {
	if user.LastName == "" {
		return user.FirstName
	}

	if user.FirstName == "" {
		return user.LastName
	}

	return user.FirstName + " " + user.LastName
}

// RegisteredTime ...
func (user *User) RegisteredTime() time.Time {
	reg, _ := time.Parse(time.RFC3339, user.Registered)
	return reg
}

// IsActive ...
func (user *User) IsActive() bool {
	// Exclude people who didn't change their nickname.
	if !user.HasNick() {
		return false
	}

	lastSeen, _ := time.Parse(time.RFC3339, user.LastSeen)
	twoWeeksAgo := time.Now().Add(-14 * 24 * time.Hour)

	if lastSeen.Unix() < twoWeeksAgo.Unix() {
		return false
	}

	if len(user.AnimeList().Items) == 0 {
		return false
	}

	return true
}

// IsPro ...
func (user *User) IsPro() bool {
	if user.ProExpires == "" {
		return false
	}

	return DateTimeUTC() < user.ProExpires
}

// ExtendProDuration ...
func (user *User) ExtendProDuration(duration time.Duration) {
	var startDate time.Time

	if user.ProExpires == "" {
		startDate = time.Now().UTC()
	} else {
		startDate, _ = time.Parse(time.RFC3339, user.ProExpires)
	}

	user.ProExpires = startDate.Add(duration).Format(time.RFC3339)
}

// TimeSinceRegistered ...
func (user *User) TimeSinceRegistered() time.Duration {
	registered, _ := time.Parse(time.RFC3339, user.Registered)
	return time.Since(registered)
}

// HasNick returns whether the user has a custom nickname.
func (user *User) HasNick() bool {
	return !strings.HasPrefix(user.Nick, "g") && !strings.HasPrefix(user.Nick, "fb") && !strings.HasPrefix(user.Nick, "t") && user.Nick != ""
}

// WebsiteURL adds https:// to the URL.
func (user *User) WebsiteURL() string {
	return "https://" + user.WebsiteShortURL()
}

// WebsiteShortURL ...
func (user *User) WebsiteShortURL() string {
	return strings.Replace(strings.Replace(user.Website, "https://", "", 1), "http://", "", 1)
}

// Link returns the URI to the user page.
func (user *User) Link() string {
	return "/+" + user.Nick
}

// CoverImageURL ...
func (user *User) CoverImageURL() string {
	return "/images/cover/default.jpg"
}

// HasAvatar ...
func (user *User) HasAvatar() bool {
	return user.Avatar.Extension != ""
}

// SmallAvatar ...
func (user *User) SmallAvatar() string {
	return "//media.notify.moe/images/avatars/small/" + user.ID + user.Avatar.Extension
}

// LargeAvatar ...
func (user *User) LargeAvatar() string {
	return "//media.notify.moe/images/avatars/large/" + user.ID + user.Avatar.Extension
}

// Gravatar ...
func (user *User) Gravatar() string {
	if user.Email == "" {
		return ""
	}

	return gravatar.SecureUrl(user.Email) + "?s=" + fmt.Sprint(AvatarMaxSize)
}

// PushSubscriptions ...
func (user *User) PushSubscriptions() *PushSubscriptions {
	subs, _ := GetPushSubscriptions(user.ID)
	return subs
}

// Inventory ...
func (user *User) Inventory() *Inventory {
	inventory, _ := GetInventory(user.ID)
	return inventory
}

// ActivateItemEffect ...
func (user *User) ActivateItemEffect(itemID string) error {
	month := 30 * 24 * time.Hour

	switch itemID {
	case "pro-account-3":
		user.ExtendProDuration(3 * month)
		user.Save()
		return nil

	case "pro-account-6":
		user.ExtendProDuration(6 * month)
		user.Save()
		return nil

	case "pro-account-12":
		user.ExtendProDuration(12 * month)
		user.Save()
		return nil

	case "pro-account-24":
		user.ExtendProDuration(24 * month)
		user.Save()
		return nil

	default:
		return errors.New("Can't activate unknown item: " + itemID)
	}
}

// SetNick changes the user's nickname safely.
func (user *User) SetNick(newName string) error {
	setNickMutex.Lock()
	defer setNickMutex.Unlock()

	newName = autocorrect.FixUserNick(newName)

	if !validator.IsValidNick(newName) {
		return errors.New("Invalid nickname")
	}

	if newName == user.Nick {
		return nil
	}

	// Make sure the nickname doesn't exist already
	_, err := GetUserByNick(newName)

	// If there was no error: the username exists.
	// If "not found" is not included in the error message it's a different error type.
	if err == nil || strings.Index(err.Error(), "not found") == -1 {
		return errors.New("Username '" + newName + "' is taken already")
	}

	user.ForceSetNick(newName)
	return nil
}

// ForceSetNick forces a nickname overwrite.
func (user *User) ForceSetNick(newName string) {
	// Delete old nick reference
	DB.Delete("NickToUser", user.Nick)

	// Set new nick
	user.Nick = newName

	DB.Set("NickToUser", user.Nick, &NickToUser{
		Nick:   user.Nick,
		UserID: user.ID,
	})
}

// SetEmail changes the user's email safely.
func (user *User) SetEmail(newName string) error {
	setEmailMutex.Lock()
	defer setEmailMutex.Unlock()

	if !validator.IsValidEmail(user.Email) {
		return errors.New("Invalid email address")
	}

	// Delete old email reference
	DB.Delete("EmailToUser", user.Email)

	// Set new email
	user.Email = newName

	DB.Set("EmailToUser", user.Email, &EmailToUser{
		Email:  user.Email,
		UserID: user.ID,
	})

	return nil
}

// RefreshOsuInfo refreshes a user's Osu information.
func (user *User) RefreshOsuInfo() error {
	if user.Accounts.Osu.Nick == "" {
		return errors.New("User doesn't have an osu username")
	}

	osu, err := osu.GetUser(user.Accounts.Osu.Nick)

	if err != nil {
		return err
	}

	user.Accounts.Osu.PP, _ = strconv.ParseFloat(osu.PPRaw, 64)
	user.Accounts.Osu.Level, _ = strconv.ParseFloat(osu.Level, 64)
	user.Accounts.Osu.Accuracy, _ = strconv.ParseFloat(osu.Accuracy, 64)

	return nil
}
