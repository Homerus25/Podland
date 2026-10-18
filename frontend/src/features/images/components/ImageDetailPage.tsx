import { Table, Tbody, Text } from "@chakra-ui/react";
import { displayTags } from "@features/images/components/imageDisplayTags";
import { useImage, useImageInspect } from "@features/images/hooks/useImages";
import type { ImageDetail } from "@features/images/services/imagesApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	isNotFound,
	KeyValueTable,
} from "@shared/components/detailKit";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useParams } from "react-router-dom";

function ImageOverview({ image }: { image: ImageDetail }) {
	return (
		<Table size="sm">
			<Tbody>
				<DetailRow label="Name">
					<Text fontFamily="mono">{displayTags(image)}</Text>
				</DetailRow>
				<DetailRow label="Id">
					<Text fontFamily="mono" wordBreak="break-all">
						{image.id}
					</Text>
				</DetailRow>
				<DetailRow label="Created">
					{formatRelativeDate(image.createdAt)}
				</DetailRow>
				<DetailRow label="User">{image.user ? image.user : "—"}</DetailRow>
				<DetailRow label="Env">
					<KeyValueTable testId="image-detail-env" values={image.env} />
				</DetailRow>
				<DetailRow label="Entrypoint">
					<Text fontFamily="mono">{image.entrypoint?.join(" ") || "—"}</Text>
				</DetailRow>
				<DetailRow label="Cmd">
					<Text fontFamily="mono">{image.cmd?.join(" ") || "—"}</Text>
				</DetailRow>
				<DetailRow label="Labels">
					<KeyValueTable testId="image-detail-labels" values={image.labels} />
				</DetailRow>
				<DetailRow label="Size">{formatBytes(image.size)}</DetailRow>
			</Tbody>
		</Table>
	);
}

// The image detail page (spec 301): the adapter over the shared DetailPage
// layout. It owns the overview query (GET .../images/{imageId}) and the
// lazy inspect query (enabled on first reveal), and hands the layout their
// structural slices; the layout owns the whole render.
export function ImageDetailPage() {
	const { machineId, imageId } = useParams<{
		machineId: string;
		imageId: string;
	}>();
	const [revealed, setRevealed] = useState(false);
	const { data: image, isPending, error } = useImage(machineId!, imageId!);
	const inspectQuery = useImageInspect(machineId!, imageId!, {
		enabled: revealed,
	});

	return (
		<DetailPage
			title={image ? displayTags(image) : imageId!}
			backTo="images"
			isPending={isPending}
			error={error}
			notFound={isNotFound(error)}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		>
			{image ? <ImageOverview image={image} /> : null}
		</DetailPage>
	);
}
