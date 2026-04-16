// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import 'dart:async';
import 'package:google_sign_in/google_sign_in.dart';

class AuthService {
  static final AuthService _instance = AuthService._internal();
  factory AuthService() => _instance;
  AuthService._internal();
  final GoogleSignIn _googleSignIn = GoogleSignIn.instance;
  GoogleSignInAccount? _currentUser;
  final StreamController<GoogleSignInAccount?> _userController =
      StreamController.broadcast();
  Stream<GoogleSignInAccount?> get onCurrentUserChanged =>
      _userController.stream;
  GoogleSignInAccount? get currentUser => _currentUser;
  String? get userEmail => currentUser?.email;
  final Completer<void> _initCompleter = Completer();
  Future<void> get initializationComplete => _initCompleter.future;
  final List<String> _appScopes = const [
    'https://www.googleapis.com/auth/cloud-platform',
    'https://www.googleapis.com/auth/cloud-platform.read-only',
    'https://www.googleapis.com/auth/userinfo.email',
    'https://www.googleapis.com/auth/userinfo.profile',
    'https://www.googleapis.com/auth/datastore', // Firestore scope
    'https://www.googleapis.com/auth/devstorage.full_control', // Cloud Storage scope
  ];

  Future<void> init({required String clientId}) async {
    _googleSignIn.authenticationEvents.listen(
      (event) {
        final GoogleSignInAccount? user = switch (event) {
          GoogleSignInAuthenticationEventSignIn() => event.user,
          GoogleSignInAuthenticationEventSignOut() => null,
        };
        _currentUser = user;
        _userController.add(_currentUser);
      },
      onError: (error) {
        print('[AuthService] Error in authenticationEvents stream: $error');
      },
    );
    await _googleSignIn.initialize(clientId: clientId);
    await _googleSignIn.attemptLightweightAuthentication();

    _initCompleter.complete();
  }

  Future<void> authenticate() async {
    if (_googleSignIn.supportsAuthenticate()) {
      try {
        final GoogleSignInAccount account = await _googleSignIn.authenticate(
          scopeHint: _appScopes,
        );
        _currentUser = account;
        _userController.add(account);
      } catch (error) {
        print('[AuthService] ERROR in _googleSignIn.authenticate(): $error');
        rethrow;
      }
    } else {
      print('[AuthService] authenticate() is not supported on this platform.');
      throw Exception('Authentication is not supported on this platform.');
    }
  }

  Future<void> signOut() {
    return _googleSignIn.signOut();
  }
  Future<bool> authorize(List<String> scopes) async {
    if (_currentUser == null) {
      return false;
    }
    try {
      await _currentUser!.authorizationClient.authorizeScopes(scopes);
      return true;
    } catch (e) {
      print("Error requesting scopes: $e");
      return false;
    }
  }

  Future<Map<String, String>?> getIDTokenHeaders() async {
    if (_currentUser == null) {
      throw Exception('No user is currently signed in.');
    }
    final GoogleSignInAuthentication auth = await _currentUser!.authentication;
    final String? idToken = auth.idToken;
    if (idToken == null) {
      throw Exception('Could not retrieve ID token. Please sign in again.');
    }
    print("ID Token: $idToken");
    return {'Authorization': 'Bearer $idToken'};
  }
  Future<Map<String, String>?> getAuthHeaders() async {
    if (_currentUser == null) {
      throw Exception('No user is currently signed in.');
    }
    final bool granted = await authorize(_appScopes);
    if (!granted) {
      throw Exception('User did not grant necessary permissions.');
    }
    final Map<String, String>? headers = await _currentUser!.authorizationClient
        .authorizationHeaders(_appScopes, promptIfNecessary: true);
    if (headers == null) {
      throw Exception('Could not retrieve auth headers. Please sign in again.');
    }
    return headers;
  }
}
