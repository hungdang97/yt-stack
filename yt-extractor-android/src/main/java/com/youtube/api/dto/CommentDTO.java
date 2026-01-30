package com.youtube.api.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class CommentDTO {
    private String commentId;
    private String commentText;
    private String authorName;
    private String authorThumbnail;
    private String authorChannelId;
    private String textualUploadDate;
    private String uploadDate;
    private Integer likeCount;
    private Integer replyCount;
    private Boolean hearted;
    private Boolean pinned;
    private Boolean verified;
    private String nextPageToken; // For replies pagination
}
