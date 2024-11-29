/*
Copyright (C) 2024 Steve Miller KC1AWV

This program is free software: you can redistribute it and/or modify it
under the terms of the GNU General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option)
any later version.

This program is distributed in the hope that it will be useful, but WITHOUT
ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
more details.

You should have received a copy of the GNU General Public License along with
this program. If not, see <http://www.gnu.org/licenses/>.
*/

package config

import (
	"encoding/json"
	"os"
)

type TargetRelay struct {
	Callsign string `json:"callsign"`
	Address  string `json:"address"`
}

type Config struct {
	LogLevel            string        `json:"log_level"`
	RelayCallsign       string        `json:"relay_callsign"`
	BindAddress         string        `json:"bind_address"`
	WebInterfaceAddress string        `json:"web_interface_address"`
	DaemonMode          bool          `json:"daemon_mode"`
	TargetRelays        []TargetRelay `json:"target_relays"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
