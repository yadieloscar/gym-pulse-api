# Research: Macro calculator and meal template

Every number the service ships is either the owner's stated value, a published equation, or a copy
of a USDA record. This file says which is which and what was and was not verified on 2026-11-16.

## Equations and constants

| Item | Value used | Source and status |
| --- | --- | --- |
| Resting energy | `10·kg + 6.25·cm − 5·age + 5` (male), `− 161` (female) | Mifflin MD, St Jeor ST, et al. "A new predictive equation for resting energy expenditure in healthy individuals." *Am J Clin Nutr* 1990;51(2):241-7. **Verified** against the citation record and two clinical calculators (QxMD, Merck Manual) that reproduce it. Derived from 498 adults aged 19–78; the paper's own full regression is 9.99/6.25/4.92 with the same constants. |
| Imperial form | `4.536·lb + 15.875·in − 5·age + s` | The owner's coefficients. They are the metric ones converted: 10 × 0.4536 and 6.25 × 2.54. Using 0.4536 rather than 0.45359237 changes resting energy by less than 0.02 kcal at 660 lb. |
| Activity multipliers | 1.375, 1.55, 1.725 | The owner's values. They are the widely used "activity factor" set (1.2 / 1.375 / 1.55 / 1.725 / 1.9). **No primary source was verified**: they are not in the Mifflin-St Jeor paper, and the search found only secondary pages attributing them to later exercise-physiology textbooks. The result is therefore labelled an estimate. |
| Sessions → level | ≤3 light, 4–5 moderate, ≥6 very | Drafting default that resolves the overlap in the owner's ranges (1–3, 3–5, 6–7). |
| Energy per gram | 4 / 4 / 9 kcal for protein / carbohydrate / fat | The owner's values; the general Atwater factors. Not re-verified in this session. |
| Protein rate | 0.70–1.10 g/lb, default 0.95 or 0.85 | The owner's values (1.54–2.43 g/kg). For comparison, the International Society of Sports Nutrition position stand (Jäger et al., *J Int Soc Sports Nutr* 2017;14:20) gives 1.4–2.0 g/kg/day for most exercising people. **Verified** that the stand states that range. The defaults (1.87 and 2.09 g/kg) sit at or just above its top. |
| Fat rate | 0.25–0.45 g/lb, default 0.33 | The owner's values. No source checked. |
| Goal adjustments | −500…−300, −300…−250, 0, +200…+400 kcal | The owner's ranges; the default is the middle of each (drafting default). |
| Carbohydrate cycling | Rest days about 17.5% under the daily figure | The owner's 15–20% range; the middle is a drafting default. The exact whole-gram value is chosen so the week's total is unchanged. |
| Meal split | Protein even; carbohydrate 25/20/35/20; fat 30/30/10/30 | Drafting default implementing "40–60 g protein per meal" (met when daily protein is 160–240 g) and "carbs concentrated around Meal 3". The same ISSN stand suggests 20–40 g (0.25 g/kg) doses every 3–4 hours; the owner's figure is higher and is used as given. |

## Foods

All facts are copied from **USDA FoodData Central, SR Legacy, release 2021-10-28**
(`FoodData_Central_sr_legacy_food_json_2021-10-28.zip`, downloaded from `fdc.nal.usda.gov`). Nine
records were first read through the FDC API and matched the file exactly. Values are per 100 g.

| Food id | FDC id | USDA description | kcal | Protein | Carb | Fat | Unit (weight, source) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `chicken_breast` | 171534 | Chicken, broiler or fryers, breast, skinless, boneless, meat only, cooked, grilled | 151 | 30.5 | 0 | 3.17 | oz (28.35 g) |
| `ground_beef_93` | 174755 | Beef, ground, 93% lean meat / 7% fat, crumbles, cooked, pan-browned | 209 | 28.9 | 0 | 9.51 | oz |
| `sirloin_steak` | 174694 | Beef, loin, top sirloin filet, boneless, separable lean only, trimmed to 0" fat, choice, cooked, grilled | 174 | 30.4 | 0 | 5.83 | oz |
| `eggs` | 173424 | Egg, whole, cooked, hard-boiled | 155 | 12.6 | 1.12 | 10.6 | count (large = 50 g, USDA portion) |
| `egg_whites` | 172183 | Egg, white, raw, fresh | 52 | 10.9 | 0.73 | 0.17 | count (large = 33 g, USDA portion) |
| `greek_yogurt` | 170894 | Yogurt, Greek, plain, nonfat | 59 | 10.2 | 3.6 | 0.39 | g (USDA lists only a 170 g container) |
| `whey` | 173180 | Beverages, Protein powder whey based | 352 | 78.1 | 6.25 | 1.56 | g |
| `sourdough_bread` | 172675 | Bread, french or vienna (includes sourdough) | 272 | 10.8 | 51.9 | 2.42 | oz (USDA's "slice" is 139 g, too large to be a useful unit) |
| `potatoes` | 170440 | Potatoes, boiled, cooked without skin, flesh, without salt | 86 | 1.71 | 20.0 | 0.1 | oz |
| `sweet_potatoes` | 168483 | Sweet potato, cooked, baked in skin, flesh, without salt | 90 | 2.01 | 20.7 | 0.15 | oz |
| `flour_tortillas` | 167535 | Tortillas, ready-to-bake or -fry, flour, shelf stable | 297 | 8.01 | 49.3 | 7.58 | count (tortilla = 49 g, USDA portion) |
| `oats` | 173904 | Cereals, oats, regular and quick, not fortified, dry | 379 | 13.2 | 67.7 | 6.52 | cup (81 g, USDA portion) |
| `banana` | 173944 | Bananas, raw | 89 | 1.09 | 22.8 | 0.33 | count (medium = 118 g, USDA portion) |
| `white_rice` | 168878 | Rice, white, long-grain, regular, enriched, cooked | 130 | 2.69 | 28.2 | 0.28 | cup (158 g, USDA portion) |
| `olive_oil` | 171413 | Oil, olive, salad or cooking | 884 | 0 | 0 | 100 | g |
| `avocado` | 171705 | Avocados, raw, all commercial varieties | 160 | 2.0 | 8.53 | 14.7 | g |
| `almonds` | 170567 | Nuts, almonds | 579 | 21.2 | 21.6 | 49.9 | oz |
| `butter` | 173410 | Butter, salted | 717 | 0.85 | 0.06 | 81.1 | g |

- **"Baby gold potatoes"**: SR Legacy has no gold-potato record, so the list uses the generic
  boiled potato record and names it "Potatoes, boiled" rather than claiming a variety.
- **"Fruit"** is one concrete food, banana, so the portion and facts are real.
- **Exclusion tags**: dairy (yogurt, whey, butter), nuts (almonds), gluten (sourdough, flour
  tortillas), eggs (eggs, egg whites), red meat (ground beef, sirloin), rice (white rice). Oats are
  not tagged gluten: oats contain no gluten themselves; cross-contact is not something this list can
  know. Chicken, potatoes, sweet potato, oats, banana, olive oil, and avocado carry no tag, so every
  role survives every combination of exclusions (pinned by a test).
- USDA energy uses food-specific factors, so a food's kcal is not exactly 4/4/9 of its macros; a
  test bounds the gap at 12% to catch a miscopied value.

## Decisions

- **One calculation owner.** The service computes; the app renders. A fourth read
  (`/calculator/options`) supplies foods, ranges, and defaults so the app holds no copy.
  Rejected: a second implementation in the app for instant sliders (two sources of truth).
- **Store the targets, rebuild the meals.** The saved row keeps the targets computed at save, so a
  later change to the maths cannot move a saved plan. Meals are rebuilt on read from those targets
  and the food list, so a corrected food fact reaches saved plans without a migration.
- **Foods in code.** 18 fixed published records need no table, seed, or read path.
- **Linear solve, not an optimizer.** Three foods and three macros is a 3×3 system.
- **Adults only (18–100).** The equation was derived from adults; the table enforces it too.

## Sources

- Mifflin-St Jeor: https://agris.fao.org/search/en/records/65de26fe7c7033e84be9ee76 (citation record); a calculator reproducing it:
  https://www.merckmanuals.com/iframe/manual/medical-calculators/mifflinstjeor.htm
- ISSN position stand: https://jissn.biomedcentral.com/articles/10.1186/s12970-017-0177-8
- USDA FoodData Central downloads: https://fdc.nal.usda.gov/download-datasets
