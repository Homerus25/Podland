import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { CreateContainerPage } from "@features/containers/components/CreateContainerPage";
import { ImageDetailPage } from "@features/images/components/ImageDetailPage";
import { NetworkDetailPage } from "@features/networks/components/NetworkDetailPage";
import { SecretDetailPage } from "@features/secrets/components/SecretDetailPage";
import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import { AdminRoute } from "@shared/components/AdminRoute";
import { Layout } from "@shared/components/Layout";
import { ProtectedRoute } from "@shared/components/ProtectedRoute";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { Navigate, Route, Routes } from "react-router-dom";
import { DashboardPage } from "@/pages/DashboardPage";
import { LoginPage } from "@/pages/LoginPage";
import { LogsPage } from "@/pages/LogsPage";
import { MachineDetailPage } from "@/pages/MachineDetailPage";
import { MachinesPage } from "@/pages/MachinesPage";
import { SetupPage } from "@/pages/SetupPage";
import { UsersPage } from "@/pages/UsersPage";

export function App() {
	return (
		<Routes>
			<Route path="/login" element={<LoginPage />} />
			<Route path="/setup" element={<SetupPage />} />
			<Route
				path="/"
				element={
					<ProtectedRoute>
						<Layout />
					</ProtectedRoute>
				}
			>
				<Route index element={<DashboardPage />} />
				<Route path="machines" element={<MachinesPage />} />
				<Route path="machines/:machineId" element={<MachineDetailPage />} />
				{/* The static `new` segment ranks above the dynamic
				`:containerId` below, so the create page and the detail page
				coexist in the containers area. */}
				<Route
					path="machines/:machineId/containers/new"
					element={<CreateContainerPage />}
				/>
				<Route
					path="machines/:machineId/containers/:containerId"
					element={<ContainerDetailPage />}
				/>
				<Route
					path="machines/:machineId/volumes/:volumeName"
					element={<VolumeDetailPage />}
				/>
				<Route
					path="machines/:machineId/images/:imageId"
					element={<ImageDetailPage />}
				/>
				<Route
					path="machines/:machineId/networks/:networkName"
					element={<NetworkDetailPage />}
				/>
				<Route
					path="machines/:machineId/secrets/:secretId"
					element={<SecretDetailPage />}
				/>
				<Route path={LOG_ROUTE_PATTERN} element={<LogsPage />} />
				<Route
					path="users"
					element={
						<AdminRoute>
							<UsersPage />
						</AdminRoute>
					}
				/>
			</Route>
			<Route path="*" element={<Navigate to="/" replace />} />
		</Routes>
	);
}
