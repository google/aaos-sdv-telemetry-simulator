/*
 *  Copyright 2025 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import 'dart:convert';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:http/http.dart' as http;
import 'package:web_demo/auth_service.dart';
import 'package:web_demo/main.dart';
import 'package:web_demo/new_simulation_page.dart';
import 'simulation_detail_page.dart';

class SimulationListPage extends StatefulWidget {
  const SimulationListPage({super.key});
  @override
  State<SimulationListPage> createState() => _SimulationListPageState();
}

class _SimulationListPageState extends State<SimulationListPage> {
  List<Map<String, dynamic>> simulations = [];
  String? errorMessage;

  // Filter and sort state variables
  String _filterText = "";
  String _sortField = "received_at";
  bool _sortAscending = false;
  final List<String> _sortOptions = [
    'id',
    'status',
    'owner',
    'received_at',
    'status_updated_at',
    'started_at',
  ];

  @override
  void initState() {
    super.initState();
    _startFirestorePolling();
  }

  void _startFirestorePolling() async {
    while (mounted) {
      try {
        await _fetchSimulations();
      } catch (e) {
        if (mounted) {
          setState(() {
            errorMessage = 'An error occurred during polling: $e';
          });
        }
        print("Polling error: $e");
      }
      await Future.delayed(const Duration(seconds: 5));
    }
  }

  Future<void> _fetchSimulations() async {
    try {
      final headers = await authService.getAuthHeaders();
      final url = Uri.parse(
        '${configService.getFirestoreApiBaseUrl()}/records',
      );
      final response = await http.get(url, headers: headers);
      if (!mounted) return;
      if (response.statusCode == 200) {
        final data = jsonDecode(response.body);
        final List<Map<String, dynamic>> fetchedSimulations =
            (data['documents'] as List? ?? [])
                .map((doc) => _parseFirestoreDocument(doc))
                .toList();
        setState(() {
          simulations = fetchedSimulations;
          errorMessage = null;
        });
      } else {
        setState(() {
          errorMessage =
              'Failed to fetch simulations (HTTP ${response.statusCode}).';
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          errorMessage = 'Error fetching simulations: $e';
        });
      }
      print("Error fetching simulations: $e");
    }
  }

  /// Parses Firestore fields into a flat Map structure.
  Map<String, dynamic> _parseFirestoreDocument(Map<String, dynamic> doc) {
    final Map<String, dynamic> fields = doc['fields'] ?? {};
    return fields.map(
      (key, value) => MapEntry(key, _parseFirestoreValue(value)),
    );
  }

  dynamic _parseFirestoreValue(Map<String, dynamic> value) {
    if (value.containsKey('timestampValue')) {
      return DateTime.parse(value['timestampValue']);
    }
    if (value.containsKey('stringValue')) return value['stringValue'];
    if (value.containsKey('integerValue')) {
      return int.tryParse(value['integerValue'].toString());
    }
    if (value.containsKey('booleanValue')) return value['booleanValue'];
    if (value.containsKey('doubleValue')) return value['doubleValue'];
    return null;
  }

  /// Applies colors based on the simulation status.
  Color _statusColor(String? status) {
    switch (status?.toLowerCase()) {
      case 'running':
        return Colors.green;
      case 'completed':
        return Colors.blue;
      case 'failed':
        return Colors.red;
      case 'cancelled':
        return Colors.orange;
      default:
        return Colors.grey;
    }
  }

  /// Formats a DateTime for display.
  String _formatDate(dynamic dt) {
    if (dt is! DateTime) return 'N/A';
    return DateFormat('yyyy-MM-dd HH:mm:ss').format(dt);
  }

  /// Formats a running duration (if applicable).
  String _calculateRunningDuration(Map<String, dynamic> sim) {
    final startedAt = sim['started_at'];
    final statusUpdatedAt = sim['status_updated_at'];
    if (startedAt is DateTime && statusUpdatedAt is DateTime) {
      final duration = statusUpdatedAt.difference(startedAt);
      return '${duration.inHours}h ${duration.inMinutes.remainder(60)}m';
    }
    return 'N/A';
  }

  @override
  Widget build(BuildContext context) {
    final filteredSimulations = simulations.where((sim) {
      final query = _filterText.toLowerCase();
      final id = (sim['id'] ?? '').toString().toLowerCase();
      final status = (sim['status'] ?? '').toString().toLowerCase();
      final owner = (sim['owner'] ?? '').toString().toLowerCase();
      return id.contains(query) ||
          status.contains(query) ||
          owner.contains(query);
    }).toList();

    filteredSimulations.sort((a, b) {
      final aVal = a[_sortField];
      final bVal = b[_sortField];
      int result;
      if (aVal == null && bVal == null) {
        result = 0;
      } else if (aVal == null)
        result = -1;
      else if (bVal == null)
        result = 1;
      else if (aVal is Comparable && bVal is Comparable)
        result = aVal.compareTo(bVal);
      else
        result = aVal.toString().compareTo(bVal.toString());
      return _sortAscending ? result : -result;
    });

    return Scaffold(
      appBar: AppBar(
        title: const Text('Simulations'),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout),
            tooltip: 'Sign Out',
            onPressed: () async {
              await authService.signOut();
            },
          ),
        ],
      ),
      body: errorMessage != null
          ? Center(child: SelectableText(errorMessage!))
          : Column(
              children: [
                Padding(
                  padding: const EdgeInsets.all(8.0),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          decoration: const InputDecoration(
                            labelText: 'Filter by ID, Status, or Owner',
                            border: OutlineInputBorder(),
                            prefixIcon: Icon(Icons.search),
                          ),
                          onChanged: (value) =>
                              setState(() => _filterText = value),
                        ),
                      ),
                      const SizedBox(width: 8),
                      DropdownButton<String>(
                        value: _sortField,
                        onChanged: (val) => setState(() => _sortField = val!),
                        items: _sortOptions
                            .map(
                              (field) => DropdownMenuItem(
                                value: field,
                                child: Text('Sort by $field'),
                              ),
                            )
                            .toList(),
                      ),
                      IconButton(
                        icon: Icon(
                          _sortAscending
                              ? Icons.arrow_upward
                              : Icons.arrow_downward,
                        ),
                        tooltip: 'Toggle Sort Order',
                        onPressed: () =>
                            setState(() => _sortAscending = !_sortAscending),
                      ),
                    ],
                  ),
                ),
                Expanded(
                  child: SingleChildScrollView(
                    scrollDirection: Axis.vertical,
                    child: SingleChildScrollView(
                      scrollDirection: Axis.horizontal,
                      child: DataTable(
                        columns: const [
                          DataColumn(label: Text('ID')),
                          DataColumn(label: Text('Status')),
                          DataColumn(label: Text('Owner')),
                          DataColumn(label: Text('Received At')),
                          DataColumn(label: Text('Status Updated At')),
                          DataColumn(label: Text('Running Duration')),
                        ],
                        rows: filteredSimulations.map<DataRow>((sim) {
                          return DataRow(
                            onSelectChanged: (selected) {
                              if (selected == true) {
                                Navigator.push(
                                  context,
                                  MaterialPageRoute(
                                    builder: (context) => SimulationDetailPage(
                                      simulationId: sim['id'] ?? '',
                                      simulationData: sim,
                                    ),
                                  ),
                                );
                              }
                            },
                            cells: [
                              DataCell(SelectableText(sim['id'] ?? 'N/A')),
                              DataCell(
                                SelectableText(
                                  sim['status'] ?? 'Unknown',
                                  style: TextStyle(
                                    color: _statusColor(sim['status']),
                                  ),
                                ),
                              ),
                              DataCell(SelectableText(sim['owner'] ?? 'N/A')),
                              DataCell(
                                SelectableText(_formatDate(sim['received_at'])),
                              ),
                              DataCell(
                                SelectableText(
                                  _formatDate(sim['status_updated_at']),
                                ),
                              ),
                              DataCell(
                                SelectableText(_calculateRunningDuration(sim)),
                              ),
                            ],
                          );
                        }).toList(),
                      ),
                    ),
                  ),
                ),
              ],
            ),
      floatingActionButton: FloatingActionButton(
        onPressed: () {
          Navigator.push(
            context,
            MaterialPageRoute(builder: (context) => const NewSimulationPage()),
          );
        },
        tooltip: 'Schedule New Simulation',
        child: const Icon(Icons.add),
      ),
    );
  }
}
