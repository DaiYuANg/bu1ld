package org.bu1ld.plugins.java;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.PropertyNamingStrategies;
import io.avaje.inject.Bean;
import io.avaje.inject.Factory;

@Factory
final class JacksonFactory {
    @Bean
    ObjectMapper objectMapper() {
        return new ObjectMapper().setPropertyNamingStrategy(PropertyNamingStrategies.SNAKE_CASE);
    }
}
