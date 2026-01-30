package com.youtube.api.config;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Contact;
import io.swagger.v3.oas.models.info.Info;
import io.swagger.v3.oas.models.info.License;
import io.swagger.v3.oas.models.servers.Server;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.List;

@Configuration
public class OpenApiConfig {

    @Bean
    public OpenAPI youtubeOpenAPI() {
        Server localServer = new Server();
        localServer.setUrl("http://localhost:8080");
        localServer.setDescription("Local Development Server");

        Server productionServer = new Server();
        productionServer.setUrl("https://your-production-url.com");
        productionServer.setDescription("Production Server");

        Contact contact = new Contact();
        contact.setName("YouTube API Support");
        contact.setEmail("support@example.com");
        contact.setUrl("https://github.com/your-repo");

        License license = new License()
                .name("GPL-3.0")
                .url("https://www.gnu.org/licenses/gpl-3.0.html");

        Info info = new Info()
                .title("YouTube Extractor API")
                .version("1.0.0")
                .description("Complete YouTube data extraction API powered by NewPipeExtractor. " +
                        "Extract video metadata, download links, comments, channel information, playlists, and more. " +
                        "No YouTube API key required!")
                .contact(contact)
                .license(license);

        return new OpenAPI()
                .info(info)
                .servers(List.of(localServer, productionServer));
    }
}
