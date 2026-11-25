import path from "node:path";
import { CoverageReport } from "monocart-coverage-reports";

const missingCoverage = "Chromium did not collect application coverage";

export const coverageReport = new CoverageReport({
  outputDir: "test-results/coverage",
  sourceFilter: "src/**",
  reports: [["lcovonly", { projectRoot: path.resolve("..") }]],
});

export default async function writeCoverageReport() {
  if (!coverageReport.hasCache()) throw new Error(missingCoverage);
  await coverageReport.generate();
}
