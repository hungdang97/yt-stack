package com.youtube.api.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class VideoInfoDTO {
    private String id;
    private String title;
    // Removed fields that require /next API (disabled for performance):
    // - description
    // - viewCount
    // - likeCount
    // - uploadDate
    private String uploaderName;
    private String uploaderUrl;
    private String thumbnailUrl;
    private Long duration;
    private List<VideoStreamDTO> videoStreams;
    private List<AudioStreamDTO> audioStreams;
    private List<String> availableAudioLanguages;
    private List<SubtitleDTO> subtitles;
    private String category;
    private List<String> tags;

}
