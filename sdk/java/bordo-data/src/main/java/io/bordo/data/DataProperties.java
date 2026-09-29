package io.bordo.data;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties("bordo.data")
public class DataProperties {

    private String url;
    private String username;
    private String password;
    private int maximumPoolSize = 10;
    private int minimumIdle = 2;
    private long connectionTimeout = 30000;
    private boolean otelEnabled = true;

    public String getUrl() { return url; }
    public void setUrl(String url) { this.url = url; }

    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }

    public String getPassword() { return password; }
    public void setPassword(String password) { this.password = password; }

    public int getMaximumPoolSize() { return maximumPoolSize; }
    public void setMaximumPoolSize(int maximumPoolSize) { this.maximumPoolSize = maximumPoolSize; }

    public int getMinimumIdle() { return minimumIdle; }
    public void setMinimumIdle(int minimumIdle) { this.minimumIdle = minimumIdle; }

    public long getConnectionTimeout() { return connectionTimeout; }
    public void setConnectionTimeout(long connectionTimeout) { this.connectionTimeout = connectionTimeout; }

    public boolean isOtelEnabled() { return otelEnabled; }
    public void setOtelEnabled(boolean otelEnabled) { this.otelEnabled = otelEnabled; }
}
