package com.youtube.api.service;

import com.youtube.api.dto.*;
import org.schabi.newpipe.extractor.InfoItem;
import org.schabi.newpipe.extractor.Page;
import org.schabi.newpipe.extractor.ServiceList;
import org.schabi.newpipe.extractor.channel.ChannelInfo;
import org.schabi.newpipe.extractor.channel.tabs.ChannelTabInfo;
import org.schabi.newpipe.extractor.comments.CommentsInfo;
import org.schabi.newpipe.extractor.comments.CommentsInfoItem;
import org.schabi.newpipe.extractor.exceptions.ExtractionException;
import org.schabi.newpipe.extractor.kiosk.KioskInfo;
import org.schabi.newpipe.extractor.linkhandler.ListLinkHandler;
import org.schabi.newpipe.extractor.playlist.PlaylistInfo;
import org.schabi.newpipe.extractor.search.SearchExtractor;
import org.schabi.newpipe.extractor.stream.AudioStream;
import org.schabi.newpipe.extractor.stream.StreamInfo;
import org.schabi.newpipe.extractor.stream.StreamInfoItem;
import org.schabi.newpipe.extractor.stream.SubtitlesStream;
import org.schabi.newpipe.extractor.stream.VideoStream;
import org.springframework.stereotype.Service;

import java.io.IOException;
import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.stream.Collectors;

@Service
public class YouTubeService {

    public VideoInfoDTO getVideoInfo(String videoId) throws IOException, ExtractionException {
        String url = "https://www.youtube.com/watch?v=" + videoId;
        StreamInfo info = StreamInfo.getInfo(url);

        // Extract unique audio languages from audio streams
        List<String> availableAudioLanguages = info.getAudioStreams().stream()
            .map(stream -> {
                String[] trackInfo = parseAudioTrackFromUrl(stream.getContent());
                return trackInfo[0]; // audioTrackId (language code)
            })
            .filter(lang -> lang != null && !lang.isEmpty())
            .distinct()
            .collect(Collectors.toList());

        return VideoInfoDTO.builder()
                .id(videoId)
                .title(info.getName())
                // Removed fields that require /next API (disabled for performance):
                // .description(info.getDescription() != null ? info.getDescription().getContent() : "")
                // .viewCount(info.getViewCount())
                // .likeCount(info.getLikeCount())
                // .uploadDate(info.getUploadDate() != null ? info.getUploadDate().offsetDateTime().toString() : null)
                .uploaderName(info.getUploaderName())
                .uploaderUrl(info.getUploaderUrl())
                .thumbnailUrl(info.getThumbnails().isEmpty() ? null : info.getThumbnails().get(0).getUrl())
                .duration(info.getDuration())
                .videoStreams(convertVideoStreams(info.getVideoStreams(), info.getVideoOnlyStreams()))
                .audioStreams(convertAudioStreams(info.getAudioStreams()))
                .availableAudioLanguages(availableAudioLanguages)
                .subtitles(convertSubtitles(info.getSubtitles()))
                .category(info.getCategory())
                .tags(info.getTags())
                .build();
    }


    public SearchResultDTO search(String query, String pageToken) throws IOException, ExtractionException {
        SearchExtractor extractor = ServiceList.YouTube.getSearchExtractor(query);
        extractor.fetchPage();

        List<InfoItem> items;
        Page nextPage;

        if (pageToken != null && !pageToken.isEmpty()) {
            // Get next page
            Page page = deserializePage(pageToken);
            var pageItems = extractor.getPage(page);
            items = pageItems.getItems();
            nextPage = pageItems.getNextPage();
        } else {
            // Get first page
            var initialPage = extractor.getInitialPage();
            items = initialPage.getItems();
            nextPage = initialPage.getNextPage();
        }

        List<SearchItemDTO> searchItems = new ArrayList<>();
        for (InfoItem item : items) {
            SearchItemDTO dto = SearchItemDTO.builder()
                    .type(item.getInfoType().name().toLowerCase())
                    .id(item.getUrl())
                    .title(item.getName())
                    .thumbnailUrl(item.getThumbnails().isEmpty() ? null : item.getThumbnails().get(0).getUrl())
                    .uploaderName(item instanceof StreamInfoItem ?
                            ((StreamInfoItem) item).getUploaderName() : null)
                    .build();

            if (item instanceof StreamInfoItem) {
                StreamInfoItem streamItem = (StreamInfoItem) item;
                dto.setDuration(streamItem.getDuration());
                dto.setViewCount(streamItem.getViewCount());
                dto.setUploadDate(streamItem.getUploadDate() != null ?
                        streamItem.getUploadDate().offsetDateTime().toString() : null);
            }

            searchItems.add(dto);
        }

        return SearchResultDTO.builder()
                .items(searchItems)
                .hasNextPage(nextPage != null)
                .nextPageToken(serializePage(nextPage))
                .build();
    }

    private List<VideoStreamDTO> convertVideoStreams(List<VideoStream> muxedStreams, List<VideoStream> videoOnlyStreams) {
        List<VideoStreamDTO> allStreams = new ArrayList<>();

        // Add muxed streams (video + audio combined) - usually 360p and below
        for (VideoStream stream : muxedStreams) {
            allStreams.add(VideoStreamDTO.builder()
                    .url(stream.getContent())
                    .quality(stream.getResolution())
                    .format(stream.getFormat() != null ? stream.getFormat().getName() : null)
                    .mimeType(stream.getFormat() != null ? stream.getFormat().getMimeType() : null)
                    .bitrate((long) stream.getBitrate())
                    .fileSize(parseFileSizeFromUrl(stream.getContent()))
                    .codec(stream.getCodec())
                    .width(stream.getWidth())
                    .height(stream.getHeight())
                    .fps(stream.getFps())
                    .videoOnly(false)
                    .build());
        }

        // Add video-only streams (adaptive) - higher qualities like 720p, 1080p, 4K
        for (VideoStream stream : videoOnlyStreams) {
            allStreams.add(VideoStreamDTO.builder()
                    .url(stream.getContent())
                    .quality(stream.getResolution())
                    .format(stream.getFormat() != null ? stream.getFormat().getName() : null)
                    .mimeType(stream.getFormat() != null ? stream.getFormat().getMimeType() : null)
                    .bitrate((long) stream.getBitrate())
                    .fileSize(parseFileSizeFromUrl(stream.getContent()))
                    .codec(stream.getCodec())
                    .width(stream.getWidth())
                    .height(stream.getHeight())
                    .fps(stream.getFps())
                    .videoOnly(true)
                    .build());
        }

        return allStreams;
    }

    private List<AudioStreamDTO> convertAudioStreams(List<AudioStream> streams) {
        return streams.stream()
                .map(stream -> {
                    String url = stream.getContent();
                    String[] audioTrackInfo = parseAudioTrackFromUrl(url);
                    return AudioStreamDTO.builder()
                            .url(url)
                            .quality(stream.getAverageBitrate() + "kbps")
                            .format(stream.getFormat() != null ? stream.getFormat().getName() : null)
                            .mimeType(stream.getFormat() != null ? stream.getFormat().getMimeType() : null)
                            .bitrate((long) stream.getAverageBitrate())
                            .fileSize(parseFileSizeFromUrl(url))
                            .codec(stream.getCodec())
                            .audioTrackId(audioTrackInfo[0])
                            .audioTrackType(audioTrackInfo[1])
                            .isOriginal("original".equals(audioTrackInfo[1]))
                            .build();
                })
                .collect(Collectors.toList());
    }


    private List<SubtitleDTO> convertSubtitles(List<SubtitlesStream> subtitles) {
        return subtitles.stream()
                .map(sub -> SubtitleDTO.builder()
                        .url(sub.getContent())
                        .languageCode(sub.getLanguageTag())
                        .format(sub.getFormat() != null ? sub.getFormat().getName() : null)
                        .autoGenerated(sub.isAutoGenerated())
                        .build())
                .collect(Collectors.toList());
    }

    // ============= NEW METHODS =============

    public RelatedVideosDTO getRelatedVideos(String videoId) throws IOException, ExtractionException {
        String url = "https://www.youtube.com/watch?v=" + videoId;
        StreamInfo info = StreamInfo.getInfo(url);

        List<SearchItemDTO> relatedVideos = info.getRelatedItems().stream()
                .filter(item -> item instanceof StreamInfoItem)
                .map(item -> {
                    StreamInfoItem streamItem = (StreamInfoItem) item;
                    return SearchItemDTO.builder()
                            .type("video")
                            .id(extractVideoId(streamItem.getUrl()))
                            .title(streamItem.getName())
                            .thumbnailUrl(streamItem.getThumbnails().isEmpty() ? null : streamItem.getThumbnails().get(0).getUrl())
                            .uploaderName(streamItem.getUploaderName())
                            .duration(streamItem.getDuration())
                            .viewCount(streamItem.getViewCount())
                            .uploadDate(streamItem.getUploadDate() != null ?
                                    streamItem.getUploadDate().offsetDateTime().toString() : null)
                            .build();
                })
                .collect(Collectors.toList());

        return RelatedVideosDTO.builder()
                .relatedVideos(relatedVideos)
                .build();
    }

    public SuggestionDTO getSuggestions(String query) throws IOException, ExtractionException {
        List<String> suggestions = ServiceList.YouTube.getSuggestionExtractor().suggestionList(query);

        return SuggestionDTO.builder()
                .query(query)
                .suggestions(suggestions)
                .build();
    }

    public CommentsPageDTO getComments(String videoId, String pageToken) throws IOException, ExtractionException {
        String url = "https://www.youtube.com/watch?v=" + videoId;

        List<CommentsInfoItem> commentItems;
        Page nextPage;

        if (pageToken != null && !pageToken.isEmpty()) {
            // Deserialize page token and get next page
            Page page = deserializePage(pageToken);
            CommentsInfo initialInfo = CommentsInfo.getInfo(url);
            var pageItems = CommentsInfo.getMoreItems(ServiceList.YouTube, initialInfo, page);

            commentItems = pageItems.getItems();
            nextPage = pageItems.getNextPage();
        } else {
            // Get first page
            CommentsInfo commentsInfo = CommentsInfo.getInfo(url);
            commentItems = commentsInfo.getRelatedItems();
            nextPage = commentsInfo.getNextPage();
        }

        List<CommentDTO> comments = commentItems.stream()
                .map(comment -> CommentDTO.builder()
                        .commentId(comment.getCommentId())
                        .commentText(comment.getCommentText() != null ? comment.getCommentText().getContent() : "")
                        .authorName(comment.getUploaderName())
                        .authorThumbnail(comment.getUploaderAvatars().isEmpty() ? null :
                                comment.getUploaderAvatars().get(0).getUrl())
                        .authorChannelId(extractChannelId(comment.getUploaderUrl()))
                        .textualUploadDate(comment.getTextualUploadDate())
                        .uploadDate(comment.getUploadDate() != null ?
                                comment.getUploadDate().offsetDateTime().toString() : null)
                        .likeCount(comment.getLikeCount())
                        .replyCount(comment.getReplyCount())
                        .hearted(comment.isHeartedByUploader())
                        .pinned(comment.isPinned())
                        .verified(comment.isUploaderVerified())
                        .build())
                .collect(Collectors.toList());

        return CommentsPageDTO.builder()
                .comments(comments)
                .nextPageToken(serializePage(nextPage))
                .hasNextPage(nextPage != null)
                .disabled(false)  // We can't easily determine this from pagination
                .build();
    }

    public CommentsPageDTO getCommentReplies(String videoId, String commentId, String pageToken)
            throws IOException, ExtractionException {
        // Note: NewPipe handles replies through the same comments extractor with a different page token
        return getComments(videoId, pageToken);
    }

    public ChannelInfoDTO getChannelInfo(String channelIdOrUrl) throws IOException, ExtractionException {
        String url;
        if (channelIdOrUrl.startsWith("http")) {
            // Full URL provided
            url = channelIdOrUrl;
        } else if (channelIdOrUrl.startsWith("@")) {
            // Handle @username format
            url = "https://www.youtube.com/" + channelIdOrUrl;
        } else if (channelIdOrUrl.startsWith("UC") || channelIdOrUrl.length() == 24) {
            // Channel ID format (usually starts with UC and is 24 chars)
            url = "https://www.youtube.com/channel/" + channelIdOrUrl;
        } else {
            // Assume it's a username without @
            url = "https://www.youtube.com/@" + channelIdOrUrl;
        }

        ChannelInfo channelInfo = ChannelInfo.getInfo(url);

        List<ChannelTabDTO> tabs = channelInfo.getTabs().stream()
                .map(tab -> ChannelTabDTO.builder()
                        .name(tab.getContentFilters().isEmpty() ? "main" : tab.getContentFilters().get(0))
                        .url(tab.getUrl())
                        .build())
                .collect(Collectors.toList());

        return ChannelInfoDTO.builder()
                .id(channelInfo.getId())
                .name(channelInfo.getName())
                .url(channelInfo.getUrl())
                .description(channelInfo.getDescription())
                .avatarUrl(channelInfo.getAvatars().stream()
                        .map(image -> image.getUrl())
                        .collect(Collectors.toList()))
                .bannerUrl(channelInfo.getBanners().stream()
                        .map(image -> image.getUrl())
                        .collect(Collectors.toList()))
                .subscriberCount(channelInfo.getSubscriberCount())
                .videoCount(-1L) // Not directly available in ChannelInfo
                .verified(channelInfo.isVerified())
                .tabs(tabs)
                .build();
    }

    public SearchResultDTO getChannelVideos(String channelId, String pageToken)
            throws IOException, ExtractionException {
        String url = "https://www.youtube.com/channel/" + channelId;

        // Get channel info to find videos tab
        ChannelInfo channelInfo = ChannelInfo.getInfo(url);

        // Find the videos tab
        ListLinkHandler videosTab = channelInfo.getTabs().stream()
                .filter(tab -> !tab.getContentFilters().isEmpty() &&
                        tab.getContentFilters().get(0).equals("videos"))
                .findFirst()
                .orElse(channelInfo.getTabs().isEmpty() ? null : channelInfo.getTabs().get(0));

        if (videosTab == null) {
            return SearchResultDTO.builder()
                    .items(new ArrayList<>())
                    .hasNextPage(false)
                    .nextPageToken(null)
                    .build();
        }

        // Get tab content
        ChannelTabInfo tabInfo;
        Page nextPage;

        if (pageToken != null && !pageToken.isEmpty()) {
            // Get next page
            Page page = deserializePage(pageToken);
            var pageItems = ChannelTabInfo.getMoreItems(ServiceList.YouTube, videosTab, page);
            tabInfo = new ChannelTabInfo(ServiceList.YouTube.getServiceId(), videosTab);
            tabInfo.setRelatedItems(pageItems.getItems());
            nextPage = pageItems.getNextPage();
        } else {
            // Get first page
            tabInfo = ChannelTabInfo.getInfo(ServiceList.YouTube, videosTab);
            nextPage = tabInfo.getNextPage();
        }

        List<SearchItemDTO> items = convertInfoItemsToSearchItems(tabInfo.getRelatedItems());

        return SearchResultDTO.builder()
                .items(items)
                .hasNextPage(nextPage != null)
                .nextPageToken(serializePage(nextPage))
                .build();
    }

    public PlaylistInfoDTO getPlaylistInfo(String playlistId, String pageToken)
            throws IOException, ExtractionException {
        String url = "https://www.youtube.com/playlist?list=" + playlistId;

        PlaylistInfo playlistInfo;
        Page nextPage;

        if (pageToken != null && !pageToken.isEmpty()) {
            // Get next page
            Page page = deserializePage(pageToken);
            playlistInfo = PlaylistInfo.getInfo(url);
            var pageItems = PlaylistInfo.getMoreItems(ServiceList.YouTube, url, page);
            playlistInfo.setRelatedItems(pageItems.getItems());
            nextPage = pageItems.getNextPage();
        } else {
            // Get first page
            playlistInfo = PlaylistInfo.getInfo(url);
            nextPage = playlistInfo.getNextPage();
        }

        List<SearchItemDTO> videos = convertInfoItemsToSearchItems(playlistInfo.getRelatedItems());

        return PlaylistInfoDTO.builder()
                .id(playlistInfo.getId())
                .name(playlistInfo.getName())
                .url(playlistInfo.getUrl())
                .thumbnailUrl(playlistInfo.getThumbnails().isEmpty() ? null :
                        playlistInfo.getThumbnails().get(0).getUrl())
                .uploaderName(playlistInfo.getUploaderName())
                .uploaderUrl(playlistInfo.getUploaderUrl())
                .uploaderAvatarUrl(playlistInfo.getUploaderAvatars().isEmpty() ? null :
                        playlistInfo.getUploaderAvatars().get(0).getUrl())
                .videoCount(playlistInfo.getStreamCount())
                .videos(videos)
                .nextPageToken(serializePage(nextPage))
                .hasNextPage(nextPage != null)
                .build();
    }

    public SearchResultDTO getTrending(String countryCode) throws IOException, ExtractionException {
        KioskInfo kioskInfo = KioskInfo.getInfo(ServiceList.YouTube, "Trending");

        List<SearchItemDTO> items = convertInfoItemsToSearchItems(kioskInfo.getRelatedItems());

        return SearchResultDTO.builder()
                .items(items)
                .hasNextPage(false)
                .nextPageToken(null)
                .build();
    }

    public SearchResultDTO getKiosk(String kioskId) throws IOException, ExtractionException {
        KioskInfo kioskInfo = KioskInfo.getInfo(ServiceList.YouTube, kioskId);

        List<SearchItemDTO> items = convertInfoItemsToSearchItems(kioskInfo.getRelatedItems());

        return SearchResultDTO.builder()
                .items(items)
                .hasNextPage(kioskInfo.hasNextPage())
                .nextPageToken(kioskInfo.getNextPage() != null ? kioskInfo.getNextPage().getUrl() : null)
                .build();
    }

    // Helper methods
    private List<SearchItemDTO> convertInfoItemsToSearchItems(List<? extends InfoItem> items) {
        return items.stream()
                .filter(item -> item instanceof StreamInfoItem)
                .map(item -> {
                    StreamInfoItem streamItem = (StreamInfoItem) item;
                    return SearchItemDTO.builder()
                            .type("video")
                            .id(extractVideoId(streamItem.getUrl()))
                            .title(streamItem.getName())
                            .thumbnailUrl(streamItem.getThumbnails().isEmpty() ? null :
                                    streamItem.getThumbnails().get(0).getUrl())
                            .uploaderName(streamItem.getUploaderName())
                            .duration(streamItem.getDuration())
                            .viewCount(streamItem.getViewCount())
                            .uploadDate(streamItem.getUploadDate() != null ?
                                    streamItem.getUploadDate().offsetDateTime().toString() : null)
                            .build();
                })
                .collect(Collectors.toList());
    }

    private String extractVideoId(String url) {
        if (url == null) return null;
        if (url.contains("v=")) {
            return url.substring(url.indexOf("v=") + 2, url.indexOf("v=") + 13);
        }
        if (url.contains("youtu.be/")) {
            return url.substring(url.indexOf("youtu.be/") + 9).split("[?&]")[0];
        }
        return url;
    }

    private String extractChannelId(String url) {
        if (url == null) return null;
        if (url.contains("/channel/")) {
            return url.substring(url.indexOf("/channel/") + 9).split("[?/]")[0];
        }
        return url;
    }

    // Helper method to serialize Page object to Base64 string
    private String serializePage(Page page) {
        if (page == null) return null;
        try {
            java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
            java.io.ObjectOutputStream oos = new java.io.ObjectOutputStream(baos);
            oos.writeObject(page);
            oos.close();
            return Base64.getEncoder().encodeToString(baos.toByteArray());
        } catch (IOException e) {
            return null;
        }
    }

    // Helper method to deserialize Page object from Base64 string
    private Page deserializePage(String pageToken) {
        if (pageToken == null || pageToken.isEmpty()) return null;
        try {
            byte[] data = Base64.getDecoder().decode(pageToken);
            java.io.ByteArrayInputStream bais = new java.io.ByteArrayInputStream(data);
            java.io.ObjectInputStream ois = new java.io.ObjectInputStream(bais);
            return (Page) ois.readObject();
        } catch (Exception e) {
            // If deserialization fails, try creating a Page from the string directly (backward compatibility)
            return new Page(pageToken);
        }
    }

    /**
     * Parse audio track info from URL xtags parameter.
     * URL contains xtags like: acont=dubbed:lang=vi or acont=original:lang=en
     * @return String array [audioTrackId (lang), audioTrackType (acont)]
     */
    private String[] parseAudioTrackFromUrl(String url) {
        String[] result = {null, null};
        if (url == null || url.isEmpty()) return result;

        try {
            // Find xtags parameter in URL
            int xtagsStart = url.indexOf("xtags=");
            if (xtagsStart == -1) return result;

            int xtagsEnd = url.indexOf("&", xtagsStart);
            String xtagsValue = xtagsEnd == -1
                    ? url.substring(xtagsStart + 6)
                    : url.substring(xtagsStart + 6, xtagsEnd);

            // URL decode the xtags value
            xtagsValue = URLDecoder.decode(xtagsValue, StandardCharsets.UTF_8);

            // Parse lang= and acont= from xtags (format: acont=original:lang=en)
            for (String part : xtagsValue.split(":")) {
                if (part.startsWith("lang=")) {
                    result[0] = part.substring(5);
                } else if (part.startsWith("acont=")) {
                    result[1] = part.substring(6);
                }
            }
        } catch (Exception e) {
            // Ignore parsing errors
        }
        return result;
    }

    /**
     * Parse file size from URL clen parameter.
     * @return file size in bytes, or null if not found
     */
    private Long parseFileSizeFromUrl(String url) {
        if (url == null || url.isEmpty()) return null;

        try {
            int clenStart = url.indexOf("clen=");
            if (clenStart == -1) return null;

            int clenEnd = url.indexOf("&", clenStart);
            String clenValue = clenEnd == -1
                    ? url.substring(clenStart + 5)
                    : url.substring(clenStart + 5, clenEnd);

            return Long.parseLong(clenValue);
        } catch (Exception e) {
            return null;
        }
    }
}
