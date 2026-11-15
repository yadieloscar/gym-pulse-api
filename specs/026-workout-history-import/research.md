# Strong and Hevy CSV export formats: research notes (2026-11-03)

Method: official help centers (Hevy read through its public Zendesk JSON API, because the HTML is behind a Cloudflare challenge), plus real export files and importer source code found on GitHub with code search. "Real" means the file looks like a user's unedited export (hundreds of rows, organic names) rather than a hand-written fixture. Raw GitHub URLs below are `https://github.com/<repo>/blob/HEAD/<path>`.

## Strong

### 1. Where the export lives
- Official: https://help.strongapp.io/article/235-export-workout-data (last updated 2021-04-27). iOS: Settings > "Export Strong Data". Android: Settings > "Export Data". It produces a "spreadsheet friendly CSV". Exported files cannot be imported back into Strong. The article documents no columns, units or size limits.

### 2. Header variants (verbatim)
**A. Strong 6.x / current, semicolon, every field quoted** (real exports dated 2019–2025):
```
"Workout #";"Date";"Workout Name";"Duration (sec)";"Exercise Name";"Set Order";"Weight (kg)";"Reps";"RPE";"Distance (meters)";"Seconds";"Notes";"Workout Notes"
```
Sources: AbishakeSrithar/fitness-tracker-backend `src/main/resources/strongData.csv` (1,624 lines, 2025); matias-ceau/biovector `data/user/_temp_raw/strong_app/strong7386152350820285603.csv` (1,608 lines, 2023–24; the file name matches Strong's export naming); mtanzim/vis-strong-go `server/fixtures/example.csv`. Ryot's importer says the delimiter is `;` on Android and `,` on iOS: IgnisDa/ryot `crates/services/importer/strong-app/src/lib.rs`. Gainalytics (`julimetzger/Gainalytics app/utils/strongCsv/strongFormat.ts`) calls this the "Strong 6.x" layout. UNCONFIRMED: whether iOS Strong 6 also writes this layout, and whether "Weight (kg)" ever becomes "Weight (lbs)". Hevy requires Strong files "in Kgs" (see below), which suggests the column is always in kg.

**B. Legacy comma layout, no unit column, RPE last** (real exports dated 2020–2022):
```
Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE
```
Sources: AlexandrosKyriakakis/StrongAppAnalytics `Data/strong.csv` (1,904 lines); harrig12/strong `strong.csv`; radupana/openweight `tools/converters/src/__fixtures__/strong-real.csv`. Text fields are quoted, numbers are not. The weight unit is not stated in the file.

**C. Semicolon layout with per-row unit columns and Workout Duration last** (real exports dated 2018–2023):
```
Date;Workout Name;Exercise Name;Set Order;Weight;Weight Unit;Reps;RPE;Distance;Distance Unit;Seconds;Notes;Workout Notes;Workout Duration
```
Sources: sitek94/strong-charts `static/example-dataset.csv`; imacek/lifting-with-friends-old `examples/android-strong.csv`. A variant without RPE and Workout Duration (2018–2019): Brandon-Valley/strong_app_export_tools `export.csv`:
```
Date;Workout Name;Exercise Name;Set Order;Weight;Weight Unit;Reps;Distance;Distance Unit;Seconds;Notes;Workout Notes
```
**D. Oldest semicolon layout, units in the header names** (2018): `Date;Workout Name;Exercise Name;Set Order;lbs;Reps;mi.;Seconds;Notes;Workout Notes` (Brandon-Valley/strong_app_export_tools `old_exports/export_0.csv`; the quoting in this file is mangled, so treat it as low-confidence).

### 3. Date and time
`YYYY-MM-DD HH:MM:SS` with no timezone, e.g. `2025-02-24 15:10:32`, in every variant. Ryot parses it as `%Y-%m-%d %H:%M:%S` and applies the server timezone. The value is the workout start and repeats on every row of that workout. Which timezone it is written in (presumably device local) is UNCONFIRMED.

### 4. Duration
- Variant A: whole seconds in `Duration (sec)`, e.g. `"2176"`.
- Variants B and C: text such as `1h 14m`, `35m`, `2h 38m`, and `38min` (harrig12/strong). Ryot's parser handles the h, m and s suffixes.

### 5. Units
- A: kg in the header (`Weight (kg)`); distance in metres (`Distance (meters)`).
- B: no unit anywhere. Importers make the user choose (openweight's `--weight-unit`).
- C: per-row `Weight Unit` (`kg`/`lbs`) and `Distance Unit` (`km`/`mi.`).
- D: unit in the header (`lbs`, `mi.`).

### 6. Set types and marker rows (variant A, biovector file)
- `Set Order` is `1`, `2`, ... for working sets, `W` for warm-up, `D` for drop set and `F` for failure.
- `W` does not use up a number. For example, Squat rows run W, 1, 2, 3.
- Marker rows are not sets: `"Rest Timer"` (rest length in `Seconds`, e.g. `120.0`) and `"Note"` (text in `Notes`). Ryot skips both and maps W/F/D to warm-up/failure/drop.
- No W/D/F markers were seen in variants B–D.

Samples (variant A):
```
"4";"2023-07-19 21:30:51";"Workout B #1409";"1742";"Squat (Barbell)";"W";"50.0";"5";"";"";"";"";""
"5";"2025-03-07 12:32:12";"Legs and Abs";"753";"Squat (Barbell)";"Rest Timer";"";"";"";"";"120.0";"";""
"2";"2025-02-25 14:50:54";"Back and Biceps";"763";"Lat Pulldown (Machine)";"Note";"";"";"";"";"";"Straight Arm Pulldown";""
```

### 7. Cardio and duration-only exercises
- Duration-only: Weight and Reps empty, Seconds filled. Example: `"14";"2025-03-24 13:06:41";"Chest and Triceps";"3686";"Plank";"1";"";"";"";"";"60.0";"";""`.
- Distance: variant C example `2019-01-20 17:03:17;"Indoor Bike";"Cycling (Indoor)";1;;;;2.4;mi.;2100;"";""`.
- In variant B, empty numbers are written as `0`. Example: `...,"Swimming",1,0,0,1.0,30,...` (StrongAppAnalytics).

### 8. Quoting, BOM, line endings
- A: every field double-quoted, `;` delimiter.
- B and C: only text fields are quoted.
- No real export with a BOM was found. openweight's `strong-bom.csv` is synthetic.
- Every real sample uses LF line endings. GitHub could have normalized these, so this is UNCONFIRMED.

## Hevy

### 1. Where the export lives
- Profile > Settings > Export & Import Data > Export Data > "Export Workouts" (or "Export Measurements"). Sources: https://help.hevyapp.com/hc/en-us/articles/43708290987415-Exporting-Your-Data-from-Hevy and https://help.hevyapp.com/hc/en-us/articles/38001424401943-How-to-Import-Strong-App-CSV-Files-and-Export-Your-Data-in-Hevy.
- The help pages document no columns.

### 2. Header (comma delimiter, text fields quoted)
```
"title","start_time","end_time","description","exercise_title","superset_id","exercise_notes","set_index","set_type","weight_kg","reps","distance_km","duration_seconds","rpe"
```
- Kilogram users. Sources: TonyTromp/HevyConnect `workouts.csv` (1,922 lines); mouadja02/Hevy-Coach `workout_data.csv`; matanabudy/workout-data-sync `examples/hevy_export_sample.csv`.
- Imperial users: `weight_kg` becomes `weight_lbs`. inkandswitch/patchwork-experiments `strength/workout_data.csv` (2026) has `weight_lbs` with `distance_km`. sheehanr/fitness-data-visualizer `data/raw/workout_data.csv` (2025) has `weight_lbs` with `distance_miles`. So the weight and distance units vary independently.
- Ryot accepts weight_kg/weight_lbs and distance_km/distance_miles (IgnisDa/ryot `crates/services/importer/hevy/src/lib.rs`).

### 3. Date and time
- Format: `D Mon YYYY, HH:MM`, 24-hour clock, day not zero-padded, hour zero-padded, no seconds, no timezone. Example: `"5 Dec 2025, 11:37"`.
- `start_time` and `end_time` repeat on every row of the workout.
- Ryot also accepts `%b %d, %Y, %I:%M %p` (e.g. `Jan 15, 2023, 8:30 AM`), presumably an older layout (UNCONFIRMED).
- Timezone: UNCONFIRMED.

### 4. Duration
- There is no workout-duration column. Derive it from `end_time` minus `start_time`.
- Per-set time is in `duration_seconds` (integer).

### 5. Units
The unit is carried only by the column-name suffix (`_kg`/`_lbs`, `_km`/`_miles`).

### 6. Set types
- `set_type` values seen in real exports: `normal`, `warmup`, `failure`, `dropset`.
- `set_index` is 0-based and counts all sets, warm-ups included.
- `superset_id` is empty or a small integer (`0`, `1`).
- Samples (inkandswitch, sheehanr):
```
"Full Body 1","8 Jun 2026, 20:13","8 Jun 2026, 21:22","","Squat (Barbell)",,"",0,"warmup",45,5,,,
"Full Body 1","8 Jun 2026, 20:13","8 Jun 2026, 21:22","","Squat (Barbell)",,"",2,"normal",230,6,,,8
"Chest/Back","19 Aug 2025, 18:11","19 Aug 2025, 18:55","barbells con mi barber","Bench Press (Barbell)",,"",2,"failure",135,3,,,10
```

### 7. Cardio
Weight and reps are empty. Distance and duration columns are filled:
`"Push","22 Sep 2025, 14:05","22 Sep 2025, 16:03","","Running",,"",0,"normal",,,5,1680,` (HevyConnect).

### 8. Quoting and line endings
- Text fields are double-quoted, including empty ones (`""`).
- Empty numeric fields are bare.
- Line endings are LF in most samples. One real lbs export (inkandswitch) has CRLF, so accept both.
- No BOM was seen.
- One sample (harshil-sri/apotheosis) has minimal quoting and a `9:56` hour, which suggests it was re-saved in a spreadsheet. Don't rely on it.

## Hevy imports Strong CSVs
- Yes: Profile > Settings > Export & Import Data > Import Data > Import Strong CSV.
- The file must be a direct, unedited Strong export, made with Strong set to English, with weights in kg.
- Only one import can be active at a time, and it can be reverted.
- Supersets and routines are not imported. Unmatched exercises become custom exercises.
- Hevy does not import its own CSVs or files from other apps. Support can send a sample Strong-format CSV.
- Sources: https://help.hevyapp.com/hc/en-us/articles/35687878672663-Importing-Strong-Data-into-Hevy (updated 2026-10-19) and article 38001424401943 above.

## File size limits
Neither Strong nor Hevy documents a limit: UNCONFIRMED.
