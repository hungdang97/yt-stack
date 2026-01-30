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
public class PlaylistInfoDTO {
    private String id;
    private String name;
    private String url;
    private String thumbnailUrl;
    private String uploaderName;
    private String uploaderUrl;
    private String uploaderAvatarUrl;
    private Long videoCount;
    private List<SearchItemDTO> videos;
    private String nextPageToken;
    private Boolean hasNextPage;
}
