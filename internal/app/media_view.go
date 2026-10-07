package app

import (
	"sort"
	"strings"
)

type mediaViewKey struct {
	Revision      uint64
	Section       string
	LibraryID     string
	Query         string
	Tab           int
	GroupEpisodes bool
}

// mediaViewCache keeps the filtered/sorted projection stable across ordinary
// UI frames. The source slices can contain thousands of catalog entries.
type mediaViewCache struct {
	key   mediaViewKey
	valid bool
	items []MediaItem
}

func (c *mediaViewCache) Get(key mediaViewKey, source []MediaItem) []MediaItem {
	if c.valid && c.key == key {
		return c.items
	}
	c.items = deriveVisibleItems(source, key)
	c.key, c.valid = key, true
	return c.items
}

func (c *mediaViewCache) Invalidate() {
	c.valid = false
	c.items = nil
}

func deriveVisibleItems(source []MediaItem, key mediaViewKey) []MediaItem {
	out := make([]MediaItem, 0, len(source))
	for _, item := range source {
		// fnOS returns the complete hierarchy for a library query. Only the
		// Movie and TV nodes belong on the library's first level; Season and
		// Episode nodes are loaded after opening their parent show.
		if item.Kind == "season" || item.Kind == "episode" && key.Section != "history" && key.Section != "favorites" {
			continue
		}
		if key.Section == "library" {
			if itemLibraryID, hasLibraryID := itemLibrary(item.Raw); hasLibraryID && itemLibraryID != key.LibraryID {
				continue
			}
		}
		if key.Section == "movies" && item.Kind != "movie" || key.Section == "tv" && item.Kind != "tv" {
			continue
		}
		if key.Section == "favorites" && !item.Favorite || key.Section == "history" && !item.Watched {
			continue
		}
		switch key.Tab {
		case 1:
			if item.Kind == "tv" {
				continue
			}
		case 2:
			if item.Kind != "tv" {
				continue
			}
		}
		out = append(out, item)
	}
	if key.Tab == 3 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].AddedAt > out[j].AddedAt })
	} else if key.Section == "home" {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Watched != out[j].Watched {
				return out[i].Watched
			}
			return out[i].AddedAt > out[j].AddedAt
		})
	}
	if key.GroupEpisodes {
		out = groupSeriesEpisodes(out)
	}
	return out
}

func groupSeriesEpisodes(items []MediaItem) []MediaItem {
	grouped := make([]MediaItem, 0, len(items))
	indices := make(map[string]int)
	for _, episode := range items {
		seriesTitle := strings.TrimSpace(episode.SeriesTitle)
		if seriesTitle == "" {
			seriesTitle = firstString(episode.Raw, "tv_name", "series_title", "seriesTitle", "show_title", "showTitle")
		}
		if episode.EpisodeNumber == 0 {
			episode.EpisodeNumber = parseMediaNumber(firstString(episode.Raw, "episode_number", "episodeNumber"))
		}
		if seriesTitle == "" {
			grouped = append(grouped, episode)
			continue
		}
		// Episode records can carry per-episode air dates in Year. Use the
		// normalized series title so those dates do not split one show into
		// dozens of cards across the catalog.
		key := strings.TrimSpace(episode.SeriesID)
		if key == "" {
			key = "title:" + strings.ToLower(strings.Join(strings.Fields(seriesTitle), " "))
		} else {
			key = "id:" + key
		}
		index, exists := indices[key]
		if !exists {
			series := episode
			series.ID = "series:" + key
			series.Title = seriesTitle
			series.SeriesTitle = seriesTitle
			series.IsSeries = true
			series.Episodes = nil
			series.Seasons = nil
			series.SeasonNumber = 0
			series.EpisodeNumber = 0
			series.Favorite, series.Watched = false, false
			series.Year = firstString(episode.Raw, "year", "release_date", "air_date")
			grouped = append(grouped, series)
			index = len(grouped) - 1
			indices[key] = index
		}
		series := &grouped[index]
		series.Episodes = append(series.Episodes, episode)
		series.Favorite = series.Favorite || episode.Favorite
		series.Watched = series.Watched || episode.Watched
	}
	return grouped
}

func normalizeMediaQuery(query string) string { return strings.TrimSpace(query) }
