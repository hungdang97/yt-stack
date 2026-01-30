package com.youtube.api.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class SearchItemDTO {
    private String type; // "video", "channel", "playlist"
    private String id;
    private String title;
    private String description;
    private String thumbnailUrl;
    private String uploaderName;
    private String uploaderUrl;
    private Long duration;
    private Long viewCount;
    private String uploadDate;
}
