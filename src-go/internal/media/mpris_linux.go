//go:build linux

package media

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mprisInterface       = "org.mpris.MediaPlayer2"
	mprisPlayerInterface = "org.mpris.MediaPlayer2.Player"
	mprisBusName         = "org.mpris.MediaPlayer2.musicd"
	mprisObjectPath      = "/org/mpris/MediaPlayer2"
)

// MPRISSession implements MPRIS media session for Linux
type MPRISSession struct {
	conn       *dbus.Conn
	handler    CommandHandler
	metadata   Metadata
	state      PlaybackState
	position   time.Duration
	shuffle    bool
	loopStatus LoopStatus
}

// NewSession creates a new MPRIS media session
func NewSession() (Session, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	// Request the MPRIS bus name
	reply, err := conn.RequestName(mprisBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to request bus name: %w", err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("bus name already taken")
	}

	session := &MPRISSession{
		conn:       conn,
		state:      StateStopped,
		shuffle:    false,
		loopStatus: LoopNone,
	}

	// Export the MPRIS interfaces
	if err := session.exportInterfaces(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to export interfaces: %w", err)
	}

	return session, nil
}

func (s *MPRISSession) exportInterfaces() error {
	// Export the main MediaPlayer2 interface
	if err := s.conn.Export(s, dbus.ObjectPath(mprisObjectPath), mprisInterface); err != nil {
		return err
	}

	// Export the Player interface
	if err := s.conn.Export(s, dbus.ObjectPath(mprisObjectPath), mprisPlayerInterface); err != nil {
		return err
	}

	// Export Properties interface
	if err := s.conn.Export(s, dbus.ObjectPath(mprisObjectPath), "org.freedesktop.DBus.Properties"); err != nil {
		return err
	}

	return nil
}

// UpdateMetadata updates the track metadata
func (s *MPRISSession) UpdateMetadata(metadata Metadata) error {
	s.metadata = metadata

	// Emit PropertiesChanged signal
	props := map[string]dbus.Variant{
		"Metadata": dbus.MakeVariant(s.getMetadataMap()),
	}

	return s.emitPropertiesChanged(mprisPlayerInterface, props)
}

// UpdatePlaybackState updates the playback state
func (s *MPRISSession) UpdatePlaybackState(state PlaybackState, position time.Duration) error {
	oldState := s.state
	s.state = state
	s.position = position

	// Only emit PlaybackStatus - clients track position based on rate
	props := map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(s.getPlaybackStatus()),
	}

	// When starting playback from stopped, emit Seeked signal with position 0
	if oldState != state && state == StatePlaying {
		// Emit Seeked signal to tell OS the current position
		s.emitSeeked(position)
	}

	return s.emitPropertiesChanged(mprisPlayerInterface, props)
}

// emitSeeked emits the Seeked signal to tell clients the current position
func (s *MPRISSession) emitSeeked(position time.Duration) error {
	return s.conn.Emit(
		dbus.ObjectPath(mprisObjectPath),
		mprisPlayerInterface+".Seeked",
		position.Microseconds(),
	)
}

// UpdateShuffle updates the shuffle state
func (s *MPRISSession) UpdateShuffle(enabled bool) error {
	s.shuffle = enabled

	props := map[string]dbus.Variant{
		"Shuffle": dbus.MakeVariant(enabled),
	}

	return s.emitPropertiesChanged(mprisPlayerInterface, props)
}

// UpdateLoopStatus updates the loop/repeat mode
func (s *MPRISSession) UpdateLoopStatus(status LoopStatus) error {
	s.loopStatus = status

	props := map[string]dbus.Variant{
		"LoopStatus": dbus.MakeVariant(string(status)),
	}

	return s.emitPropertiesChanged(mprisPlayerInterface, props)
}

// SetCommandHandler sets the handler for media commands
func (s *MPRISSession) SetCommandHandler(handler CommandHandler) {
	s.handler = handler
}

// Close releases resources
func (s *MPRISSession) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// MPRIS DBus method implementations

// org.mpris.MediaPlayer2 methods

func (s *MPRISSession) Raise() *dbus.Error {
	return nil
}

func (s *MPRISSession) Quit() *dbus.Error {
	return nil
}

// org.mpris.MediaPlayer2.Player methods

func (s *MPRISSession) Play() *dbus.Error {
	if s.handler != nil {
		s.handler.OnCommand(CmdPlay, nil)
	}
	return nil
}

func (s *MPRISSession) Pause() *dbus.Error {
	if s.handler != nil {
		s.handler.OnCommand(CmdPause, nil)
	}
	return nil
}

func (s *MPRISSession) PlayPause() *dbus.Error {
	if s.state == StatePlaying {
		return s.Pause()
	}
	return s.Play()
}

func (s *MPRISSession) Stop() *dbus.Error {
	if s.handler != nil {
		s.handler.OnCommand(CmdStop, nil)
	}
	return nil
}

func (s *MPRISSession) Next() *dbus.Error {
	if s.handler != nil {
		s.handler.OnCommand(CmdNext, nil)
	}
	return nil
}

func (s *MPRISSession) Previous() *dbus.Error {
	if s.handler != nil {
		s.handler.OnCommand(CmdPrevious, nil)
	}
	return nil
}

func (s *MPRISSession) Seek(offset int64) *dbus.Error {
	if s.handler != nil {
		newPos := s.position + time.Duration(offset)*time.Microsecond
		if newPos < 0 {
			newPos = 0
		}
		s.handler.OnCommand(CmdSeek, newPos)
	}
	return nil
}

func (s *MPRISSession) SetPosition(trackId dbus.ObjectPath, position int64) *dbus.Error {
	if s.handler != nil {
		s.handler.OnCommand(CmdSeek, time.Duration(position)*time.Microsecond)
	}
	return nil
}

// org.freedesktop.DBus.Properties methods

func (s *MPRISSession) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	switch iface {
	case mprisInterface:
		return s.getMediaPlayer2Property(prop)
	case mprisPlayerInterface:
		return s.getPlayerProperty(prop)
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown interface: %s", iface))
}

func (s *MPRISSession) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	switch iface {
	case mprisInterface:
		return s.getAllMediaPlayer2Properties(), nil
	case mprisPlayerInterface:
		return s.getAllPlayerProperties(), nil
	}
	return nil, dbus.MakeFailedError(fmt.Errorf("unknown interface: %s", iface))
}

func (s *MPRISSession) Set(iface, prop string, value dbus.Variant) *dbus.Error {
	if iface != mprisPlayerInterface {
		return nil
	}

	switch prop {
	case "Shuffle":
		enabled, ok := value.Value().(bool)
		if !ok {
			return dbus.MakeFailedError(fmt.Errorf("invalid type for Shuffle"))
		}
		s.shuffle = enabled
		if s.handler != nil {
			s.handler.OnCommand(CmdSetShuffle, enabled)
		}
	case "LoopStatus":
		status, ok := value.Value().(string)
		if !ok {
			return dbus.MakeFailedError(fmt.Errorf("invalid type for LoopStatus"))
		}
		s.loopStatus = LoopStatus(status)
		if s.handler != nil {
			s.handler.OnCommand(CmdSetLoopStatus, LoopStatus(status))
		}
	}

	return nil
}

func (s *MPRISSession) getMediaPlayer2Property(prop string) (dbus.Variant, *dbus.Error) {
	switch prop {
	case "CanQuit":
		return dbus.MakeVariant(false), nil
	case "CanRaise":
		return dbus.MakeVariant(false), nil
	case "HasTrackList":
		return dbus.MakeVariant(false), nil
	case "Identity":
		return dbus.MakeVariant("musicd"), nil
	case "DesktopEntry":
		return dbus.MakeVariant("musicd"), nil
	case "SupportedUriSchemes":
		return dbus.MakeVariant([]string{"file"}), nil
	case "SupportedMimeTypes":
		return dbus.MakeVariant([]string{"audio/mpeg", "audio/flac", "audio/x-m4a"}), nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown property: %s", prop))
}

func (s *MPRISSession) getPlayerProperty(prop string) (dbus.Variant, *dbus.Error) {
	switch prop {
	case "PlaybackStatus":
		return dbus.MakeVariant(s.getPlaybackStatus()), nil
	case "Metadata":
		return dbus.MakeVariant(s.getMetadataMap()), nil
	case "Position":
		return dbus.MakeVariant(s.position.Microseconds()), nil
	case "Rate":
		return dbus.MakeVariant(1.0), nil
	case "MinimumRate":
		return dbus.MakeVariant(1.0), nil
	case "MaximumRate":
		return dbus.MakeVariant(1.0), nil
	case "CanGoNext":
		return dbus.MakeVariant(true), nil
	case "CanGoPrevious":
		return dbus.MakeVariant(true), nil
	case "CanPlay":
		return dbus.MakeVariant(true), nil
	case "CanPause":
		return dbus.MakeVariant(true), nil
	case "CanSeek":
		return dbus.MakeVariant(true), nil
	case "CanControl":
		return dbus.MakeVariant(true), nil
	case "Volume":
		return dbus.MakeVariant(1.0), nil
	case "Shuffle":
		return dbus.MakeVariant(s.shuffle), nil
	case "LoopStatus":
		return dbus.MakeVariant(string(s.loopStatus)), nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown property: %s", prop))
}

func (s *MPRISSession) getAllMediaPlayer2Properties() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"CanQuit":             dbus.MakeVariant(false),
		"CanRaise":            dbus.MakeVariant(false),
		"HasTrackList":        dbus.MakeVariant(false),
		"Identity":            dbus.MakeVariant("musicd"),
		"DesktopEntry":        dbus.MakeVariant("musicd"),
		"SupportedUriSchemes": dbus.MakeVariant([]string{"file"}),
		"SupportedMimeTypes":  dbus.MakeVariant([]string{"audio/mpeg", "audio/flac", "audio/x-m4a"}),
	}
}

func (s *MPRISSession) getAllPlayerProperties() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(s.getPlaybackStatus()),
		"Metadata":       dbus.MakeVariant(s.getMetadataMap()),
		"Position":       dbus.MakeVariant(s.position.Microseconds()),
		"Rate":           dbus.MakeVariant(1.0),
		"MinimumRate":    dbus.MakeVariant(1.0),
		"MaximumRate":    dbus.MakeVariant(1.0),
		"CanGoNext":      dbus.MakeVariant(true),
		"CanGoPrevious":  dbus.MakeVariant(true),
		"CanPlay":        dbus.MakeVariant(true),
		"CanPause":       dbus.MakeVariant(true),
		"CanSeek":        dbus.MakeVariant(true),
		"CanControl":     dbus.MakeVariant(true),
		"Volume":         dbus.MakeVariant(1.0),
		"Shuffle":        dbus.MakeVariant(s.shuffle),
		"LoopStatus":     dbus.MakeVariant(string(s.loopStatus)),
	}
}

func (s *MPRISSession) getPlaybackStatus() string {
	switch s.state {
	case StatePlaying:
		return "Playing"
	case StatePaused:
		return "Paused"
	default:
		return "Stopped"
	}
}

func (s *MPRISSession) getMetadataMap() map[string]dbus.Variant {
	m := make(map[string]dbus.Variant)

	m["mpris:trackid"] = dbus.MakeVariant(dbus.ObjectPath("/org/musicd/track/1"))

	if s.metadata.Title != "" {
		m["xesam:title"] = dbus.MakeVariant(s.metadata.Title)
	}
	if s.metadata.Artist != "" {
		m["xesam:artist"] = dbus.MakeVariant([]string{s.metadata.Artist})
	}
	if s.metadata.Album != "" {
		m["xesam:album"] = dbus.MakeVariant(s.metadata.Album)
	}
	if s.metadata.Duration > 0 {
		m["mpris:length"] = dbus.MakeVariant(s.metadata.Duration.Microseconds())
	}
	if s.metadata.ArtPath != "" {
		m["mpris:artUrl"] = dbus.MakeVariant("file://" + s.metadata.ArtPath)
	}

	return m
}

func (s *MPRISSession) emitPropertiesChanged(iface string, props map[string]dbus.Variant) error {
	return s.conn.Emit(
		dbus.ObjectPath(mprisObjectPath),
		"org.freedesktop.DBus.Properties.PropertiesChanged",
		iface,
		props,
		[]string{},
	)
}
