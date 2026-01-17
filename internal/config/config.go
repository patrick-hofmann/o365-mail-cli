package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	ConfigDirName    = ".o365-mail-cli"
	ConfigFileName   = "config"
	AccountsFileName = "accounts.yaml"
)

// Config holds all configuration options
type Config struct {
	ClientID       string `mapstructure:"client_id"`
	CurrentAccount string `mapstructure:"current_account"`
	IMAPServer     string `mapstructure:"imap_server"`
	IMAPPort       int    `mapstructure:"imap_port"`
	SMTPServer     string `mapstructure:"smtp_server"`
	SMTPPort       int    `mapstructure:"smtp_port"`
	CacheDir       string `mapstructure:"cache_dir"`
	Debug          bool   `mapstructure:"debug"`
}

// Account represents a logged-in O365 account
type Account struct {
	Email   string    `yaml:"email"`
	AddedAt time.Time `yaml:"added_at"`
	Alias   string    `yaml:"alias,omitempty"`
}

// AccountList holds all logged-in accounts
type AccountList struct {
	Accounts []Account `yaml:"accounts"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ClientID:   "5aa6d895-1072-41c4-beb6-d8e3fdf0e7cd",
		IMAPServer: "outlook.office365.com",
		IMAPPort:   993,
		SMTPServer: "smtp.office365.com",
		SMTPPort:   587,
		CacheDir:   filepath.Join(home, ConfigDirName),
		Debug:      false,
	}
}

// Load loads the configuration from file and environment variables
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Config directory
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil // Use defaults
	}

	configDir := filepath.Join(home, ConfigDirName)

	// Configure viper
	viper.SetConfigName(ConfigFileName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir)
	viper.AddConfigPath(".")

	// Environment variables
	viper.SetEnvPrefix("O365")
	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("client_id", cfg.ClientID)
	viper.SetDefault("current_account", cfg.CurrentAccount)
	viper.SetDefault("imap_server", cfg.IMAPServer)
	viper.SetDefault("imap_port", cfg.IMAPPort)
	viper.SetDefault("smtp_server", cfg.SMTPServer)
	viper.SetDefault("smtp_port", cfg.SMTPPort)
	viper.SetDefault("cache_dir", cfg.CacheDir)
	viper.SetDefault("debug", cfg.Debug)

	// Read config file (if exists)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// No config file is OK
	}

	// Unmarshal into struct
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}

	return cfg, nil
}

// Save saves the configuration
func Save(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ConfigDirName)

	// Create directory
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Set values
	viper.Set("client_id", cfg.ClientID)
	viper.Set("current_account", cfg.CurrentAccount)
	viper.Set("imap_server", cfg.IMAPServer)
	viper.Set("imap_port", cfg.IMAPPort)
	viper.Set("smtp_server", cfg.SMTPServer)
	viper.Set("smtp_port", cfg.SMTPPort)
	viper.Set("cache_dir", cfg.CacheDir)
	viper.Set("debug", cfg.Debug)

	// Save
	configPath := filepath.Join(configDir, ConfigFileName+".yaml")
	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetConfigDir returns the configuration directory
func GetConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDirName)
}

// SetValue sets a single configuration value
func SetValue(key, value string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	switch key {
	case "client_id":
		cfg.ClientID = value
	case "current_account":
		cfg.CurrentAccount = value
	case "imap_server":
		cfg.IMAPServer = value
	case "smtp_server":
		cfg.SMTPServer = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return Save(cfg)
}

// GetValue gets a single configuration value
func GetValue(key string) (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}

	switch key {
	case "client_id":
		return cfg.ClientID, nil
	case "current_account":
		return cfg.CurrentAccount, nil
	case "imap_server":
		return cfg.IMAPServer, nil
	case "smtp_server":
		return cfg.SMTPServer, nil
	case "cache_dir":
		return cfg.CacheDir, nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

// getAccountsFilePath returns the path to accounts.yaml
func getAccountsFilePath() string {
	return filepath.Join(GetConfigDir(), AccountsFileName)
}

// LoadAccounts loads the list of all logged-in accounts
func LoadAccounts() ([]Account, error) {
	path := getAccountsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Account{}, nil
		}
		return nil, fmt.Errorf("failed to read accounts file: %w", err)
	}

	var accountList AccountList
	if err := yaml.Unmarshal(data, &accountList); err != nil {
		return nil, fmt.Errorf("failed to parse accounts file: %w", err)
	}

	return accountList.Accounts, nil
}

// SaveAccounts saves the list of all accounts
func SaveAccounts(accounts []Account) error {
	configDir := GetConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	accountList := AccountList{Accounts: accounts}
	data, err := yaml.Marshal(&accountList)
	if err != nil {
		return fmt.Errorf("failed to marshal accounts: %w", err)
	}

	path := getAccountsFilePath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write accounts file: %w", err)
	}

	return nil
}

// AddAccount adds a new account or updates an existing one
func AddAccount(email string) error {
	accounts, err := LoadAccounts()
	if err != nil {
		return err
	}

	// Check if account already exists
	for i, acc := range accounts {
		if acc.Email == email {
			accounts[i].AddedAt = time.Now()
			return SaveAccounts(accounts)
		}
	}

	// Add new account
	accounts = append(accounts, Account{
		Email:   email,
		AddedAt: time.Now(),
	})

	return SaveAccounts(accounts)
}

// RemoveAccount removes an account from the list
func RemoveAccount(email string) error {
	accounts, err := LoadAccounts()
	if err != nil {
		return err
	}

	newAccounts := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc.Email != email {
			newAccounts = append(newAccounts, acc)
		}
	}

	return SaveAccounts(newAccounts)
}

// RemoveAllAccounts removes all accounts
func RemoveAllAccounts() error {
	return SaveAccounts([]Account{})
}

// SetCurrentAccount sets the active account
func SetCurrentAccount(email string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.CurrentAccount = email
	return Save(cfg)
}

// AccountExists checks if an account exists
func AccountExists(email string) bool {
	accounts, err := LoadAccounts()
	if err != nil {
		return false
	}

	for _, acc := range accounts {
		if acc.Email == email {
			return true
		}
	}
	return false
}

// GetFirstAccount returns the first account (if no current is set)
func GetFirstAccount() string {
	accounts, err := LoadAccounts()
	if err != nil || len(accounts) == 0 {
		return ""
	}
	return accounts[0].Email
}
