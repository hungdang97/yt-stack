package com.youtube.api.controller;

import com.youtube.api.config.ProxyConfig;
import com.youtube.api.config.ProxyContext;
import com.youtube.api.dto.*;
import com.youtube.api.service.YouTubeService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import io.swagger.v3.oas.annotations.media.Content;
import io.swagger.v3.oas.annotations.media.Schema;
import io.swagger.v3.oas.annotations.responses.ApiResponse;
import io.swagger.v3.oas.annotations.responses.ApiResponses;
import org.schabi.newpipe.extractor.exceptions.ExtractionException;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

@RestController
@RequestMapping("/api/youtube")
@CrossOrigin(origins = "*")
@Tag(name = "YouTube API", description = "YouTube video, channel, playlist, and search operations")
public class YouTubeController {

    @Autowired
    private YouTubeService youtubeService;

    @Operation(summary = "Get video information", description = "Retrieve complete video metadata including download links for video, audio streams and subtitles")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Successfully retrieved video information",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = VideoInfoDTO.class))),
            @ApiResponse(responseCode = "400", description = "Invalid proxy format"),
            @ApiResponse(responseCode = "500", description = "Internal server error or video not found")
    })
    @GetMapping("/video/{videoId}")
    public ResponseEntity<?> getVideoInfo(
            @Parameter(description = "YouTube video ID (e.g., dQw4w9WgXcQ)", required = true)
            @PathVariable String videoId,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            // Set proxy if provided
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            VideoInfoDTO videoInfo = youtubeService.getVideoInfo(videoId);
            return ResponseEntity.ok(videoInfo);
        } catch (IllegalArgumentException e) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST)
                    .body(createErrorResponse("Invalid proxy format: " + e.getMessage()));
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            // Always clear proxy context after request
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Search YouTube", description = "Search for videos, channels, and playlists on YouTube")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Search results retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = SearchResultDTO.class))),
            @ApiResponse(responseCode = "400", description = "Invalid search query or proxy format"),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/search")
    public ResponseEntity<?> search(
            @Parameter(description = "Search query", required = true)
            @RequestParam String q,
            @Parameter(description = "Page token for pagination", required = false)
            @RequestParam(required = false) String page,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            // Set proxy if provided
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            SearchResultDTO results = youtubeService.search(q, page);
            return ResponseEntity.ok(results);
        } catch (IllegalArgumentException e) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST)
                    .body(createErrorResponse("Invalid proxy format: " + e.getMessage()));
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            // Always clear proxy context after request
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Health check", description = "Check if the YouTube API service is running")
    @ApiResponse(responseCode = "200", description = "Service is healthy")
    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        Map<String, String> response = new HashMap<>();
        response.put("status", "UP");
        response.put("service", "YouTube API");
        return ResponseEntity.ok(response);
    }

    // ==================== NEW ENDPOINTS ====================

    @Operation(summary = "Get related videos", description = "Get videos related to a specific video (recommendations)")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Related videos retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = RelatedVideosDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/video/{videoId}/related")
    public ResponseEntity<?> getRelatedVideos(
            @Parameter(description = "YouTube video ID", required = true)
            @PathVariable String videoId,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            RelatedVideosDTO relatedVideos = youtubeService.getRelatedVideos(videoId);
            return ResponseEntity.ok(relatedVideos);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get search suggestions", description = "Get autocomplete suggestions for a search query")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Suggestions retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = SuggestionDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/suggest")
    public ResponseEntity<?> getSuggestions(
            @Parameter(description = "Search query to get suggestions for", required = true)
            @RequestParam String q,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            SuggestionDTO suggestions = youtubeService.getSuggestions(q);
            return ResponseEntity.ok(suggestions);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get video comments", description = "Get comments for a specific video with pagination support")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Comments retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = CommentsPageDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/video/{videoId}/comments")
    public ResponseEntity<?> getComments(
            @Parameter(description = "YouTube video ID", required = true)
            @PathVariable String videoId,
            @Parameter(description = "Page token for pagination", required = false)
            @RequestParam(required = false) String page,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            CommentsPageDTO comments = youtubeService.getComments(videoId, page);
            return ResponseEntity.ok(comments);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get channel information", description = "Get detailed information about a YouTube channel")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Channel information retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = ChannelInfoDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error or channel not found")
    })
    @GetMapping("/channel/{channelId}")
    public ResponseEntity<?> getChannelInfo(
            @Parameter(description = "YouTube channel ID or @username", required = true)
            @PathVariable String channelId,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            ChannelInfoDTO channelInfo = youtubeService.getChannelInfo(channelId);
            return ResponseEntity.ok(channelInfo);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get channel videos", description = "Get videos from a specific channel with pagination")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Channel videos retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = SearchResultDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/channel/{channelId}/videos")
    public ResponseEntity<?> getChannelVideos(
            @Parameter(description = "YouTube channel ID", required = true)
            @PathVariable String channelId,
            @Parameter(description = "Page token for pagination", required = false)
            @RequestParam(required = false) String page,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            SearchResultDTO videos = youtubeService.getChannelVideos(channelId, page);
            return ResponseEntity.ok(videos);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get playlist information", description = "Get complete playlist information with all videos")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Playlist information retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = PlaylistInfoDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error or playlist not found")
    })
    @GetMapping("/playlist/{playlistId}")
    public ResponseEntity<?> getPlaylistInfo(
            @Parameter(description = "YouTube playlist ID", required = true)
            @PathVariable String playlistId,
            @Parameter(description = "Page token for pagination", required = false)
            @RequestParam(required = false) String page,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            PlaylistInfoDTO playlistInfo = youtubeService.getPlaylistInfo(playlistId, page);
            return ResponseEntity.ok(playlistInfo);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get trending videos", description = "Get currently trending videos on YouTube")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Trending videos retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = SearchResultDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/trending")
    public ResponseEntity<?> getTrending(
            @Parameter(description = "Country code (e.g., US, GB, VN)", required = false)
            @RequestParam(required = false, defaultValue = "US") String country,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            SearchResultDTO trending = youtubeService.getTrending(country);
            return ResponseEntity.ok(trending);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    @Operation(summary = "Get kiosk content", description = "Get content from YouTube kiosks (Live, Music, Gaming, etc.)")
    @ApiResponses(value = {
            @ApiResponse(responseCode = "200", description = "Kiosk content retrieved successfully",
                    content = @Content(mediaType = "application/json", schema = @Schema(implementation = SearchResultDTO.class))),
            @ApiResponse(responseCode = "500", description = "Internal server error")
    })
    @GetMapping("/kiosk/{kioskId}")
    public ResponseEntity<?> getKiosk(
            @Parameter(description = "Kiosk ID: Live, TrendingMusic, TrendingGaming, TrendingMovies, TrendingPodcasts", required = true)
            @PathVariable String kioskId,
            @Parameter(description = "Optional proxy in format username:password:ip:port", required = false)
            @RequestParam(required = false) String proxy) {
        try {
            if (proxy != null && !proxy.trim().isEmpty()) {
                ProxyConfig proxyConfig = ProxyConfig.parse(proxy);
                ProxyContext.setProxy(proxyConfig);
            }

            SearchResultDTO kiosk = youtubeService.getKiosk(kioskId);
            return ResponseEntity.ok(kiosk);
        } catch (IOException | ExtractionException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                    .body(createErrorResponse(e.getMessage()));
        } finally {
            ProxyContext.clear();
        }
    }

    private Map<String, String> createErrorResponse(String message) {
        Map<String, String> error = new HashMap<>();
        error.put("error", message);
        return error;
    }
}
