// Package scanner provides library scanning functionality.
// This file handles parsing of NFO metadata files (XML format).
package scanner

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

// NFO file types we look for
const (
	ArtistNFO = "artist.nfo"
	AlbumNFO  = "album.nfo"
)

// ArtistNFO represents metadata from an artist.nfo file
type ArtistInfo struct {
	Name          string   `xml:"name" json:"name"`
	SortName      string   `xml:"sortname" json:"sortName,omitempty"`
	MusicBrainzID string   `xml:"musicbrainzartistid" json:"musicBrainzId,omitempty"`
	Rating        float64  `xml:"rating" json:"rating,omitempty"`
	Biography     string   `xml:"biography" json:"biography,omitempty"`
	Outline       string   `xml:"outline" json:"outline,omitempty"`
	Genre         []string `xml:"genre" json:"genres,omitempty"`
	Style         []string `xml:"style" json:"styles,omitempty"`
	Mood          []string `xml:"mood" json:"moods,omitempty"`
	Born          string   `xml:"born" json:"born,omitempty"`
	Formed        string   `xml:"formed" json:"formed,omitempty"`
	Died          string   `xml:"died" json:"died,omitempty"`
	Disbanded     string   `xml:"disbanded" json:"disbanded,omitempty"`
	YearsActive   string   `xml:"yearsactive" json:"yearsActive,omitempty"`
	Thumb         []Thumb  `xml:"thumb" json:"thumbs,omitempty"`
	Fanart        *Fanart  `xml:"fanart" json:"fanart,omitempty"`
	Path          string   `xml:"-" json:"path"` // Path to the NFO file
}

// AlbumNFO represents metadata from an album.nfo file
type AlbumInfo struct {
	Title               string   `xml:"title" json:"title"`
	Artist              string   `xml:"artist" json:"artist,omitempty"`
	ArtistDesc          string   `xml:"artistdesc" json:"artistDesc,omitempty"`
	MusicBrainzAlbumID  string   `xml:"musicbrainzalbumid" json:"musicBrainzAlbumId,omitempty"`
	MusicBrainzArtistID string   `xml:"musicbrainzartistid" json:"musicBrainzArtistId,omitempty"`
	ReleaseType         string   `xml:"releasetype" json:"releaseType,omitempty"`
	Year                int      `xml:"year" json:"year,omitempty"`
	Rating              float64  `xml:"rating" json:"rating,omitempty"`
	UserRating          float64  `xml:"userrating" json:"userRating,omitempty"`
	Votes               int      `xml:"votes" json:"votes,omitempty"`
	Genre               []string `xml:"genre" json:"genres,omitempty"`
	Style               []string `xml:"style" json:"styles,omitempty"`
	Mood                []string `xml:"mood" json:"moods,omitempty"`
	Theme               []string `xml:"theme" json:"themes,omitempty"`
	Label               string   `xml:"label" json:"label,omitempty"`
	Type                string   `xml:"type" json:"type,omitempty"`
	Compilation         bool     `xml:"compilation" json:"compilation,omitempty"`
	Review              string   `xml:"review" json:"review,omitempty"`
	Thumb               []Thumb  `xml:"thumb" json:"thumbs,omitempty"`
	Path                string   `xml:"-" json:"path"`      // Path to the NFO file
	AlbumPath           string   `xml:"-" json:"albumPath"` // Path to the album directory
}

// Thumb represents a thumbnail/artwork reference
type Thumb struct {
	Preview string `xml:"preview,attr" json:"preview,omitempty"`
	Aspect  string `xml:"aspect,attr" json:"aspect,omitempty"`
	Type    string `xml:"type,attr" json:"type,omitempty"`
	URL     string `xml:",chardata" json:"url,omitempty"`
}

// Fanart represents fanart/background images
type Fanart struct {
	Thumb []Thumb `xml:"thumb" json:"thumbs,omitempty"`
}

// ParseArtistNFO parses an artist.nfo file
func ParseArtistNFO(path string) (*ArtistInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var artist ArtistInfo
	if err := xml.Unmarshal(data, &artist); err != nil {
		return nil, err
	}

	artist.Path = path
	return &artist, nil
}

// ParseAlbumNFO parses an album.nfo file
func ParseAlbumNFO(path string) (*AlbumInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var album AlbumInfo
	if err := xml.Unmarshal(data, &album); err != nil {
		return nil, err
	}

	album.Path = path
	album.AlbumPath = filepath.Dir(path)
	return &album, nil
}

// FindArtwork looks for common artwork files in a directory
func FindArtwork(dir string) map[string]string {
	artwork := make(map[string]string)

	// Common artwork filenames
	artworkFiles := []struct {
		names []string
		key   string
	}{
		{[]string{"folder.jpg", "folder.png", "cover.jpg", "cover.png", "front.jpg", "front.png"}, "cover"},
		{[]string{"artist.jpg", "artist.png"}, "artist"},
		{[]string{"fanart.jpg", "fanart.png", "background.jpg", "background.png"}, "fanart"},
		{[]string{"banner.jpg", "banner.png"}, "banner"},
		{[]string{"logo.png", "clearlogo.png"}, "logo"},
		{[]string{"thumb.jpg", "thumb.png"}, "thumb"},
		{[]string{"disc.png", "cdart.png"}, "disc"},
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return artwork
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := strings.ToLower(entry.Name())
		for _, af := range artworkFiles {
			for _, candidate := range af.names {
				if name == candidate {
					artwork[af.key] = filepath.Join(dir, entry.Name())
					break
				}
			}
		}
	}

	return artwork
}

// LibraryMetadata holds all pre-processed metadata found during a scan
type LibraryMetadata struct {
	Artists []ArtistInfo        `json:"artists"`
	Albums  []AlbumInfo         `json:"albums"`
	Artwork map[string][]string `json:"artwork"` // path -> list of artwork files
}

// ScanMetadata scans a library path for NFO files and artwork
func (s *Scanner) ScanMetadata(libraryPath string) (*LibraryMetadata, error) {
	meta := &LibraryMetadata{
		Artists: []ArtistInfo{},
		Albums:  []AlbumInfo{},
		Artwork: make(map[string][]string),
	}

	err := filepath.WalkDir(libraryPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip hidden directories
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != libraryPath {
				return filepath.SkipDir
			}

			// Look for artwork in this directory
			artwork := FindArtwork(path)
			if len(artwork) > 0 {
				artList := make([]string, 0, len(artwork))
				for _, artPath := range artwork {
					artList = append(artList, artPath)
				}
				meta.Artwork[path] = artList
			}

			return nil
		}

		name := strings.ToLower(d.Name())

		// Parse artist.nfo
		if name == ArtistNFO {
			artist, err := ParseArtistNFO(path)
			if err == nil && artist != nil {
				meta.Artists = append(meta.Artists, *artist)
			}
		}

		// Parse album.nfo
		if name == AlbumNFO {
			album, err := ParseAlbumNFO(path)
			if err == nil && album != nil {
				meta.Albums = append(meta.Albums, *album)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return meta, nil
}
