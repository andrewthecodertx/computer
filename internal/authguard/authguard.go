// Package authguard owns administrative handlers. Session resolution lives in
// auth.AuthMiddleware: real identity governs administration; effective identity
// governs all per-user data queries.
package authguard
