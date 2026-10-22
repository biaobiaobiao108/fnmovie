package app

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo"
	overlay "github.com/egoist/mygo/fnmovieoverlay"
	"github.com/egoist/mygo/ui"
)

// appState owns state shared by MyGo's UI callbacks. Domain state is grouped
// by lifecycle so page-specific changes stay easier to audit.
type appState struct {
	home          homeState
	window        *mygo.Window
	settings      Settings
	serverAddress string
	server        *Server
	player        *Player
	icon          *ui.Bitmap
	displayScale  float64
	posters       *PosterLoader

	navigationUIState
	sessionUIState
	catalogUIState
	detailUIState
	taskbarUIState
	playbackUIState
	personUIState
}

type navigationUIState struct {
	section          string
	query            string
	grid             ui.GridState
	catalogScroll    ui.ScrollState
	catalogScrollKey string
	sidebarList      ui.ListState
	sidebarScroll    ui.ScrollState
	selectedTab      int
}

type sessionUIState struct {
	username    string
	password    string
	status      string
	busy        bool
	loginCancel context.CancelFunc
	loggedIn    bool
	loginOpen   bool
}

type catalogUIState struct {
	libraryLoading  bool
	libraryCancel   context.CancelFunc
	libraryErr      string
	libraries       []MediaLibrary
	libraryID       string
	items           []MediaItem
	catalogs        map[string]*CatalogState
	catalogCache    *CatalogCache
	mediaView       mediaViewCache
	dataRevision    uint64
	searchChangedAt time.Time
	searchPending   bool
}

type detailUIState struct {
	detailList            ui.ListState
	detailScroll          ui.ScrollState
	detailSynopsisScroll  ui.ScrollState
	detailSynopsisOpen    bool
	selected              *MediaItem
	detailRequest         uint64
	detailCancel          context.CancelFunc
	seriesEpisodeCancel   context.CancelFunc
	seriesCastCancel      context.CancelFunc
	seriesRootCastLoading bool
	seriesLoading         bool
	seriesError           string
	castLoading           bool
	castError             string
	selectedSeasonID      string
	seriesEpisodes        []MediaItem
	seriesEpisodeCache    map[string][]MediaItem
	seriesEpisodeLoading  bool
	seriesEpisodeError    string
	seriesEpisodeRequest  uint64
	seriesRootCast        []CastMember
	seriesRootCastError   string
	seriesCastRequest     uint64
	favoritePending       map[string]bool
}

type taskbarUIState struct {
	taskbarProgress          float64
	taskbarProgressUpdatedAt time.Time
	taskbarProgressSet       bool
}

type playbackUIState struct {
	playerProxy                  *PlaybackProxy
	playback                     PlaybackState
	playingItem                  *MediaItem
	playerOverlayWindow          *mygo.Window
	playerHeaderWindow           *mygo.Window
	playerSeekFeedbackWindow     *mygo.Window
	playerMenuWindow             *mygo.Window
	playerOverlayContent         *overlay.Content
	playerHeaderContent          *overlay.Content
	playerSeekFeedbackContent    *overlay.Content
	playerMenuContent            *overlay.Content
	playerSeekFeedbackVisible    bool
	fullscreenMotion             playerFullscreenMotion
	playerOverlayHooks           bool
	playerOverlayMonitorDone     chan struct{}
	overlayMu                    sync.Mutex
	playerOverlayVisible         bool
	playerOverlayPinned          bool
	playerOverlaySeeking         bool
	playerOverlayLastInput       time.Time
	playerOverlayMenu            string
	playerMenuTracks             []PlayerTrack
	playerOverlayTransition      uint64
	playerOverlayOpacity         float64
	playerOverlayAnimationStart  time.Time
	playerOverlayAnimationFrom   float64
	playerOverlayAnimationTarget float64
	seekSliderPosition           float64
	seekDragging                 bool
	seekFeedback                 seekFeedback
	volumeDragging               bool
	playbackLoading              bool
	playbackLoadingID            string
	playbackLoadingTitle         string
	playbackCancel               context.CancelFunc
	playbackRequest              uint64
	playbackTrackCancel          context.CancelFunc
}

type personUIState struct {
	personScroll          ui.ScrollState
	selectedPerson        *CastMember
	personFavoriteLoading bool
	personItems           []MediaItem
	personLoading         bool
	personCached          bool
	personError           string
	personRequest         uint64
	personCancel          context.CancelFunc
	personCache           *personItemsCache
	personGrid            ui.GridState
}
