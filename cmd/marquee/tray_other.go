//go:build !darwin

package main

import "errors"

func runTray() error { return errors.New("the menu-bar app is macOS only; use marquee serve") }

func inAppBundle() bool { return false }

func loginEnabled() bool { return false }

func setLogin(bool) error { return errors.New("open at login is only available in the macOS app") }
