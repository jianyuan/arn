package arn

import (
	"encoding/json"
	"errors"

	"github.com/aerogo/aero"
	"github.com/aerogo/api"
)

// Actions
func init() {
	API.RegisterActions("PushSubscriptions", []*api.Action{
		// Add subscription
		&api.Action{
			Route: "/add",
			Run: func(obj interface{}, ctx *aero.Context) error {
				subscriptions := obj.(*PushSubscriptions)

				// Parse body
				body, err := ctx.Request().Body().Bytes()

				if err != nil {
					return err
				}

				var subscription *PushSubscription
				err = json.Unmarshal(body, &subscription)

				if err != nil {
					return err
				}

				// Add subscription
				err = subscriptions.Add(subscription)

				if err != nil {
					return err
				}

				subscriptions.Save()

				return nil
			},
		},

		// RemoveQuote subscription
		&api.Action{
			Route: "/remove",
			Run: func(obj interface{}, ctx *aero.Context) error {
				subscriptions := obj.(*PushSubscriptions)

				// Parse body
				body, err := ctx.Request().Body().Bytes()

				if err != nil {
					return err
				}

				var subscription *PushSubscription
				err = json.Unmarshal(body, &subscription)

				if err != nil {
					return err
				}

				// RemoveQuote subscription
				if !subscriptions.Remove(subscription.ID()) {
					return errors.New("PushSubscription does not exist")
				}

				subscriptions.Save()

				return nil
			},
		},
	})
}

// Add adds a subscription to the list if it hasn't been added yet.
func (list *PushSubscriptions) Add(subscription *PushSubscription) error {
	if list.Contains(subscription.ID()) {
		return errors.New("PushSubscription " + subscription.ID() + " has already been added")
	}

	subscription.Created = DateTimeUTC()

	list.Items = append(list.Items, subscription)

	return nil
}

// RemoveQuote removes the subscription ID from the list.
func (list *PushSubscriptions) Remove(subscriptionID string) bool {
	for index, item := range list.Items {
		if item.ID() == subscriptionID {
			list.Items = append(list.Items[:index], list.Items[index+1:]...)
			return true
		}
	}

	return false
}

// Contains checks if the list contains the subscription ID already.
func (list *PushSubscriptions) Contains(subscriptionID string) bool {
	for _, item := range list.Items {
		if item.ID() == subscriptionID {
			return true
		}
	}

	return false
}

// Authorize returns an error if the given API request is not authorized.
func (list *PushSubscriptions) Authorize(ctx *aero.Context, action string) error {
	return AuthorizeIfLoggedInAndOwnData(ctx, "id")
}

// Save saves the push subscriptions in the database.
func (list *PushSubscriptions) Save() {
	DB.Set("PushSubscriptions", list.UserID, list)
}
