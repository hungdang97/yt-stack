package com.youtube.api.config;

import jakarta.annotation.PostConstruct;
import org.schabi.newpipe.extractor.NewPipe;
import org.schabi.newpipe.extractor.localization.ContentCountry;
import org.schabi.newpipe.extractor.localization.Localization;
import org.springframework.context.annotation.Configuration;
import org.springframework.beans.factory.annotation.Autowired;

@Configuration
public class NewPipeConfig {

    @Autowired
    private DownloaderImpl downloader;

    @PostConstruct
    public void init() {
        NewPipe.init(downloader, new Localization("en", "US"), ContentCountry.DEFAULT);
    }
}
