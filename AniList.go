package arn

import (
	"strconv"
	"strings"

	"github.com/animenotifier/anilist"
	"github.com/fatih/color"
)

// FindAniListAnime tries to find an AniListAnime in our Anime database.
func FindAniListAnime(search *anilist.Anime, allAnime []*Anime) *Anime {
	match, err := GetAniListToAnime(strconv.Itoa(search.ID))

	if err == nil {
		anime, _ := GetAnime(match.AnimeID)
		return anime
	}

	if err != nil && !strings.Contains(err.Error(), "not found") {
		color.Red(err.Error())
		return nil
	}

	var mostSimilar *Anime
	var similarity float64

	for _, anime := range allAnime {
		anime.Title.Japanese = strings.Replace(anime.Title.Japanese, "2ndシーズン", "2", 1)
		anime.Title.Romaji = strings.Replace(anime.Title.Romaji, " 2nd Season", " 2", 1)
		search.TitleJapanese = strings.TrimSpace(strings.Replace(search.TitleJapanese, "2ndシーズン", "2", 1))
		search.TitleRomaji = strings.TrimSpace(strings.Replace(search.TitleRomaji, " 2nd Season", " 2", 1))

		titleSimilarity := StringSimilarity(anime.Title.Romaji, search.TitleRomaji)

		if strings.ToLower(anime.Title.Japanese) == strings.ToLower(search.TitleJapanese) {
			titleSimilarity += 1.0
		}

		if strings.ToLower(anime.Title.Romaji) == strings.ToLower(search.TitleRomaji) {
			titleSimilarity += 1.0
		}

		if strings.ToLower(anime.Title.English) == strings.ToLower(search.TitleEnglish) {
			titleSimilarity += 1.0
		}

		if titleSimilarity > similarity {
			mostSimilar = anime
			similarity = titleSimilarity
		}
	}

	if mostSimilar.EpisodeCount != search.TotalEpisodes {
		similarity -= 0.02
	}

	if similarity >= 0.92 && mostSimilar.GetMapping("anilist/anime") == "" {
		// fmt.Printf("MATCH:    %s => %s (%.2f)\n", search.TitleRomaji, mostSimilar.Title.Romaji, similarity)
		mostSimilar.AddMapping("anilist/anime", strconv.Itoa(search.ID), "")
		PanicOnError(mostSimilar.Save())
		return mostSimilar
	}

	// color.Red("MISMATCH: %s => %s (%.2f)", search.TitleRomaji, mostSimilar.Title.Romaji, similarity)

	return nil
}
