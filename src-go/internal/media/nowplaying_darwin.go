//go:build darwin

package media

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework MediaPlayer -framework AppKit

#import <Foundation/Foundation.h>
#import <MediaPlayer/MediaPlayer.h>
#import <AppKit/AppKit.h>

void updateNowPlayingInfo(const char* title, const char* artist, const char* album, double duration, const char* artPath);
void updatePlaybackState(int state, double position);
void setupRemoteCommandCenter(void);

// Forward declarations for Go callbacks
extern void goMediaCommandPlay();
extern void goMediaCommandPause();
extern void goMediaCommandTogglePlayPause();
extern void goMediaCommandStop();
extern void goMediaCommandNext();
extern void goMediaCommandPrevious();

static inline void updateNowPlayingInfoImpl(const char* title, const char* artist, const char* album, double duration, const char* artPath) {
    @autoreleasepool {
        NSMutableDictionary *nowPlayingInfo = [NSMutableDictionary dictionary];

        if (title != NULL) {
            nowPlayingInfo[MPMediaItemPropertyTitle] = [NSString stringWithUTF8String:title];
        }
        if (artist != NULL) {
            nowPlayingInfo[MPMediaItemPropertyArtist] = [NSString stringWithUTF8String:artist];
        }
        if (album != NULL) {
            nowPlayingInfo[MPMediaItemPropertyAlbumTitle] = [NSString stringWithUTF8String:album];
        }
        if (duration > 0) {
            nowPlayingInfo[MPMediaItemPropertyPlaybackDuration] = @(duration);
        }

        // Handle album art - load from file path if provided
        if (artPath != NULL) {
            NSString *path = [NSString stringWithUTF8String:artPath];
            NSImage *image = [[NSImage alloc] initWithContentsOfFile:path];
            if (image != nil) {
                // Create MPMediaItemArtwork with the image
                MPMediaItemArtwork *artwork = [[MPMediaItemArtwork alloc]
                    initWithBoundsSize:image.size
                    requestHandler:^NSImage * _Nonnull(CGSize size) {
                        return image;
                    }];
                nowPlayingInfo[MPMediaItemPropertyArtwork] = artwork;
            }
        }

        [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nowPlayingInfo];
    }
}

static inline void updatePlaybackStateImpl(int state, double position) {
    @autoreleasepool {
        NSMutableDictionary *nowPlayingInfo = [[[MPNowPlayingInfoCenter defaultCenter] nowPlayingInfo] mutableCopy];
        if (nowPlayingInfo == nil) {
            nowPlayingInfo = [NSMutableDictionary dictionary];
        }

        nowPlayingInfo[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(position);
        nowPlayingInfo[MPNowPlayingInfoPropertyPlaybackRate] = @(state == 1 ? 1.0 : 0.0);

        [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nowPlayingInfo];
    }
}

static inline void setupRemoteCommandCenterImpl() {
    @autoreleasepool {
        MPRemoteCommandCenter *center = [MPRemoteCommandCenter sharedCommandCenter];

        // Play command
        [center.playCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            goMediaCommandPlay();
            return MPRemoteCommandHandlerStatusSuccess;
        }];
        center.playCommand.enabled = YES;

        // Pause command
        [center.pauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            goMediaCommandPause();
            return MPRemoteCommandHandlerStatusSuccess;
        }];
        center.pauseCommand.enabled = YES;

        // Toggle play/pause command (for headphone button, etc.)
        [center.togglePlayPauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            goMediaCommandTogglePlayPause();
            return MPRemoteCommandHandlerStatusSuccess;
        }];
        center.togglePlayPauseCommand.enabled = YES;

        // Stop command
        [center.stopCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            goMediaCommandStop();
            return MPRemoteCommandHandlerStatusSuccess;
        }];
        center.stopCommand.enabled = YES;

        // Next track command
        [center.nextTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            goMediaCommandNext();
            return MPRemoteCommandHandlerStatusSuccess;
        }];
        center.nextTrackCommand.enabled = YES;

        // Previous track command
        [center.previousTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            goMediaCommandPrevious();
            return MPRemoteCommandHandlerStatusSuccess;
        }];
        center.previousTrackCommand.enabled = YES;
    }
}

void updateNowPlayingInfo(const char* title, const char* artist, const char* album, double duration, const char* artPath) {
    updateNowPlayingInfoImpl(title, artist, album, duration, artPath);
}

void updatePlaybackState(int state, double position) {
    updatePlaybackStateImpl(state, position);
}

void setupRemoteCommandCenter() {
    setupRemoteCommandCenterImpl();
}
*/
import "C"

import (
	"log"
	"time"
	"unsafe"
)

// Global handler for callbacks from Objective-C
var globalHandler CommandHandler

// DarwinSession implements macOS Now Playing integration
type DarwinSession struct {
	handler CommandHandler
}

// Go callback functions called from Objective-C
//
//export goMediaCommandPlay
func goMediaCommandPlay() {
	if globalHandler != nil {
		log.Printf("[MEDIA-MAC] Received Play command from media keys")
		globalHandler.OnCommand(CmdPlay, nil)
	}
}

//export goMediaCommandPause
func goMediaCommandPause() {
	if globalHandler != nil {
		log.Printf("[MEDIA-MAC] Received Pause command from media keys")
		globalHandler.OnCommand(CmdPause, nil)
	}
}

//export goMediaCommandTogglePlayPause
func goMediaCommandTogglePlayPause() {
	if globalHandler != nil {
		log.Printf("[MEDIA-MAC] Received PlayPause command from media keys")
		globalHandler.OnCommand(CmdPlayPause, nil)
	}
}

//export goMediaCommandStop
func goMediaCommandStop() {
	if globalHandler != nil {
		log.Printf("[MEDIA-MAC] Received Stop command from media keys")
		globalHandler.OnCommand(CmdStop, nil)
	}
}

//export goMediaCommandNext
func goMediaCommandNext() {
	if globalHandler != nil {
		log.Printf("[MEDIA-MAC] Received Next command from media keys")
		globalHandler.OnCommand(CmdNext, nil)
	}
}

//export goMediaCommandPrevious
func goMediaCommandPrevious() {
	if globalHandler != nil {
		log.Printf("[MEDIA-MAC] Received Previous command from media keys")
		globalHandler.OnCommand(CmdPrevious, nil)
	}
}

// NewSession creates a new macOS media session
func NewSession() (Session, error) {
	session := &DarwinSession{}
	// Setup remote command center for media key handling
	C.setupRemoteCommandCenter()
	log.Printf("[MEDIA-MAC] Remote command center initialized")
	return session, nil
}

// UpdateMetadata updates the Now Playing info
func (s *DarwinSession) UpdateMetadata(metadata Metadata) error {
	var cTitle, cArtist, cAlbum, cArtPath *C.char

	if metadata.Title != "" {
		cTitle = C.CString(metadata.Title)
		defer C.free(unsafe.Pointer(cTitle))
	}
	if metadata.Artist != "" {
		cArtist = C.CString(metadata.Artist)
		defer C.free(unsafe.Pointer(cArtist))
	}
	if metadata.Album != "" {
		cAlbum = C.CString(metadata.Album)
		defer C.free(unsafe.Pointer(cAlbum))
	}
	if metadata.ArtPath != "" {
		cArtPath = C.CString(metadata.ArtPath)
		defer C.free(unsafe.Pointer(cArtPath))
	}

	C.updateNowPlayingInfo(cTitle, cArtist, cAlbum, C.double(metadata.Duration.Seconds()), cArtPath)
	return nil
}

// UpdatePlaybackState updates the playback state
func (s *DarwinSession) UpdatePlaybackState(state PlaybackState, position time.Duration) error {
	C.updatePlaybackState(C.int(state), C.double(position.Seconds()))
	return nil
}

// UpdateShuffle updates the shuffle state
// Note: macOS Now Playing Center doesn't have native shuffle display support
func (s *DarwinSession) UpdateShuffle(enabled bool) error {
	log.Printf("[MEDIA-MAC] Shuffle state: %v (not displayed in macOS Now Playing)", enabled)
	return nil
}

// UpdateLoopStatus updates the loop/repeat mode
// Note: macOS Now Playing Center doesn't have native repeat display support
func (s *DarwinSession) UpdateLoopStatus(status LoopStatus) error {
	log.Printf("[MEDIA-MAC] Loop status: %s (not displayed in macOS Now Playing)", status)
	return nil
}

// SetCommandHandler sets the handler for media commands
func (s *DarwinSession) SetCommandHandler(handler CommandHandler) {
	s.handler = handler
	globalHandler = handler
	log.Printf("[MEDIA-MAC] Command handler registered")
}

// Close releases resources
func (s *DarwinSession) Close() error {
	return nil
}
