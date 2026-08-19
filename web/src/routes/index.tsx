import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { Spin } from 'antd';
import AppLayout from '../components/AppLayout';

// Route-level code splitting: each page is its own chunk so the initial
// download stays small. Components shared across pages (AppLayout) are
// imported eagerly.
const DashboardPage = lazy(() => import('../pages/DashboardPage'));
const SubmissionsPage = lazy(() => import('../pages/SubmissionsPage'));
const SubmissionDetailPage = lazy(() => import('../pages/SubmissionDetailPage'));
const DispatchesPage = lazy(() => import('../pages/DispatchesPage'));
const RulesPage = lazy(() => import('../pages/RulesPage'));
const AuditPage = lazy(() => import('../pages/AuditPage'));

/** Central route configuration for the application. */
export default function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<AppLayout />}>
        <Route
          index
          element={
            <Suspense fallback={<PageFallback />}>
              <DashboardPage />
            </Suspense>
          }
        />
        <Route
          path="submissions"
          element={
            <Suspense fallback={<PageFallback />}>
              <SubmissionsPage />
            </Suspense>
          }
        />
        <Route
          path="submissions/:id"
          element={
            <Suspense fallback={<PageFallback />}>
              <SubmissionDetailPage />
            </Suspense>
          }
        />
        <Route
          path="dispatches"
          element={
            <Suspense fallback={<PageFallback />}>
              <DispatchesPage />
            </Suspense>
          }
        />
        <Route
          path="rules"
          element={
            <Suspense fallback={<PageFallback />}>
              <RulesPage />
            </Suspense>
          }
        />
        <Route
          path="audit"
          element={
            <Suspense fallback={<PageFallback />}>
              <AuditPage />
            </Suspense>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}

function PageFallback() {
  return (
    <div style={{ textAlign: 'center', padding: 48 }}>
      <Spin tip="Loading…" />
    </div>
  );
}
